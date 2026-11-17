package helper

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

// streamKeepAliveWriter owns preparation/retry pings until the first business
// write. Pings use the original writer so they do not prevent a safe retry.
type streamKeepAliveWriter struct {
	gin.ResponseWriter
	mutex       sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	pingContext *gin.Context
	started     *atomic.Bool
}

func (w *streamKeepAliveWriter) stop() {
	w.cancel()
	<-w.done
}

// StopStreamKeepAlive lets response-copy helpers join preparation pings
// before mutating headers without importing the relay helper package.
func (w *streamKeepAliveWriter) StopStreamKeepAlive() { w.stop() }

func (w *streamKeepAliveWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *streamKeepAliveWriter) Write(data []byte) (int, error) {
	w.started.Store(true)
	w.stop()
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.ResponseWriter.Write(data)
}

func (w *streamKeepAliveWriter) WriteString(data string) (int, error) {
	w.started.Store(true)
	w.stop()
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.ResponseWriter.WriteString(data)
}

func (w *streamKeepAliveWriter) WriteHeader(status int) {
	// Gin only stores the status here; WriteHeaderNow/Write/Flush commits it.
	w.stop()
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.ResponseWriter.WriteHeader(status)
}

func (w *streamKeepAliveWriter) WriteHeaderNow() {
	w.started.Store(true)
	w.stop()
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.ResponseWriter.WriteHeaderNow()
}

func (w *streamKeepAliveWriter) Flush() {
	w.started.Store(true)
	w.stop()
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.ResponseWriter.Flush()
}

func (w *streamKeepAliveWriter) Status() int {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.ResponseWriter.Status()
}

func (w *streamKeepAliveWriter) Size() int {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.ResponseWriter.Size()
}

func (w *streamKeepAliveWriter) Written() bool {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.ResponseWriter.Written()
}

// ResponseStarted distinguishes business output from preparation pings. All
// ordinary writers retain the existing Written() retry protection.
func ResponseStarted(c *gin.Context) bool {
	if c == nil || c.Writer == nil {
		return false
	}
	if writer, ok := c.Writer.(*streamKeepAliveWriter); ok {
		return writer.started.Load()
	}
	return c.Writer.Written()
}

// StopStreamKeepAlive joins the pinger before changing response headers or
// handing writes to a stream scanner with its own write mutex.
func StopStreamKeepAlive(c *gin.Context) {
	if c != nil {
		if writer, ok := c.Writer.(*streamKeepAliveWriter); ok {
			writer.stop()
		}
	}
}

// StartStreamKeepAlive waits one interval before committing headers. Nested
// upstream attempts reuse the request pinger and cannot stop it on return.
func StartStreamKeepAlive(c *gin.Context, interval time.Duration) func() {
	if c == nil || c.Writer == nil || c.Request == nil || requestContextDone(c) {
		return func() {}
	}
	started := &atomic.Bool{}
	if previous, ok := c.Writer.(*streamKeepAliveWriter); ok {
		if previous.started.Load() {
			return func() {}
		}
		select {
		case <-previous.done:
			c.Writer = previous.ResponseWriter
			started = previous.started
		default:
			return func() {}
		}
	} else {
		started.Store(c.Writer.Written())
		if started.Load() {
			return func() {}
		}
	}
	if interval <= 0 {
		interval = DefaultPingInterval
	}
	SetEventStreamHeaders(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Minute)
	pingContext := c.Copy()
	pingContext.Writer = c.Writer
	writer := &streamKeepAliveWriter{ResponseWriter: c.Writer, cancel: cancel, done: make(chan struct{}), pingContext: pingContext, started: started}
	c.Writer = writer
	go writer.keepAlive(ctx, interval)
	return writer.stop
}

func (w *streamKeepAliveWriter) keepAlive(ctx context.Context, interval time.Duration) {
	defer close(w.done)
	defer w.cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.LogDebug(w.pingContext, "SSE ping goroutine panic recovered: %v", recovered)
		}
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			if err := w.ping(); err != nil {
				return
			}
		}
	}
}

func (w *streamKeepAliveWriter) ping() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	ExtendWriteDeadline(w.pingContext)
	return PingData(w.pingContext)
}
