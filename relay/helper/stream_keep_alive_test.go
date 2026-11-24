package helper

import (
	"context"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStreamKeepAliveStatusOnlyDoesNotBlockReplay(t *testing.T) {
	for _, test := range []struct {
		name     string
		interval time.Duration
		ping     bool
	}{
		{name: "before heartbeat", interval: time.Hour},
		{name: "after heartbeat", interval: time.Millisecond, ping: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			stop := StartStreamKeepAlive(c, test.interval)
			defer stop()
			writer := c.Writer.(*streamKeepAliveWriter)
			if test.ping {
				require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
			}

			c.Status(http.StatusBadRequest)
			require.False(t, ResponseStarted(c), "setting a Gin status does not commit business output")
			require.Equal(t, test.ping, c.Writer.Written())
			select {
			case <-writer.done:
			default:
				t.Fatal("status update returned before keepalive stopped")
			}

			restart := StartStreamKeepAlive(c, time.Hour)
			defer restart()
			require.NotSame(t, writer, c.Writer, "status-only output must still allow a later attempt")
			c.Writer.WriteHeaderNow()
			require.True(t, ResponseStarted(c), "committing headers must block later attempts")
		})
	}
}

func TestStreamKeepAliveBusinessWritesStopAndBlockReplay(t *testing.T) {
	for _, test := range []struct {
		name  string
		write func(*gin.Context)
	}{
		{"Write", func(c *gin.Context) { _, _ = c.Writer.Write([]byte("data: output\n\n")) }},
		{"WriteString", func(c *gin.Context) { _, _ = c.Writer.WriteString("data: output\n\n") }},
		{"WriteHeaderNow", func(c *gin.Context) { c.Writer.WriteHeaderNow() }},
		{"Flush", func(c *gin.Context) { c.Writer.Flush() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			stop := StartStreamKeepAlive(c, time.Millisecond)
			defer stop()
			writer := c.Writer.(*streamKeepAliveWriter)
			require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
			require.False(t, ResponseStarted(c), "a heartbeat does not emit business output")
			test.write(c)
			require.True(t, ResponseStarted(c))
			select {
			case <-writer.done:
			default:
				t.Fatal("business write returned before keepalive stopped")
			}
			StartStreamKeepAlive(c, time.Millisecond)()
			require.Same(t, writer, c.Writer, "business output must prevent restarting preparation pings")
		})
	}
}

func TestStreamKeepAliveNestedLifetimeAndRestart(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	outer := StartStreamKeepAlive(c, time.Hour)
	writer := c.Writer.(*streamKeepAliveWriter)
	inner := StartStreamKeepAlive(c, time.Millisecond)
	inner()
	require.Same(t, writer, c.Writer)
	select {
	case <-writer.done:
		t.Fatal("upstream attempt cleanup stopped the request keepalive")
	default:
	}
	outer()
	stop := StartStreamKeepAlive(c, time.Millisecond)
	defer stop()
	require.NotSame(t, writer, c.Writer)
	require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
	require.False(t, ResponseStarted(c), "restarting after an attempt must preserve ping-only state")
}

func TestStreamKeepAliveCancellationJoinsWriter(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	stop := StartStreamKeepAlive(c, time.Millisecond)
	writer := c.Writer.(*streamKeepAliveWriter)
	require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
	cancel()
	stop()
	select {
	case <-writer.done:
	default:
		t.Fatal("cancelled request left a running pinger")
	}
	require.False(t, ResponseStarted(c))
}

type headerCommitWriter struct {
	*httptest.ResponseRecorder
	headerStarted chan struct{}
	once          sync.Once
}

func (w *headerCommitWriter) WriteHeader(code int) {
	// Let the response handler start while the first keepalive snapshots headers.
	w.once.Do(func() { close(w.headerStarted) })
	w.ResponseRecorder.WriteHeader(code)
}

func TestStreamKeepAliveJSONResponseHeaderHandoff(t *testing.T) {
	for _, test := range []struct {
		name        string
		interval    time.Duration
		waitForPing bool
	}{
		{name: "before first keepalive", interval: time.Hour},
		{name: "during header commit", interval: time.Millisecond, waitForPing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := &headerCommitWriter{
				ResponseRecorder: httptest.NewRecorder(),
				headerStarted:    make(chan struct{}),
			}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			stop := StartStreamKeepAlive(c, test.interval)
			defer stop()
			keepAlive := c.Writer.(*streamKeepAliveWriter)
			if test.waitForPing {
				select {
				case <-writer.headerStarted:
				case <-time.After(time.Second):
					t.Fatal("keepalive did not start")
				}
			}

			response := &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": {"application/json"},
					"X-Upstream":   {"buffered"},
				},
			}
			service.IOCopyBytesGracefully(c, response, []byte(`{"ok":true}`))
			select {
			case <-keepAlive.done:
			default:
				t.Fatal("buffered response returned before keepalive stopped")
			}
			require.True(t, ResponseStarted(c))
			assert.True(t, strings.HasSuffix(writer.Body.String(), `{"ok":true}`))
			if !test.waitForPing {
				assert.Equal(t, "application/json", writer.Result().Header.Get("Content-Type"))
				assert.Equal(t, "buffered", writer.Result().Header.Get("X-Upstream"))
				assert.Equal(t, `{"ok":true}`, writer.Body.String())
			}
		})
	}
}

func TestStreamScannerHandler_RequestKeepAliveHandoff(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled, previousInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 60
	t.Cleanup(func() {
		settings.PingIntervalEnabled, settings.PingIntervalSeconds = previousEnabled, previousInterval
	})

	for _, test := range []struct {
		name        string
		interval    time.Duration
		waitForPing bool
	}{
		{name: "before first keepalive", interval: time.Hour},
		{name: "during header commit", interval: time.Millisecond, waitForPing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := &headerCommitWriter{
				ResponseRecorder: httptest.NewRecorder(),
				headerStarted:    make(chan struct{}),
			}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			stop := StartStreamKeepAlive(c, test.interval)
			defer stop()
			if test.waitForPing {
				select {
				case <-writer.headerStarted:
				case <-time.After(time.Second):
					t.Fatal("keepalive did not start")
				}
			}

			resp := &http.Response{
				Header: http.Header{
					"X-Reasoning-Included": {"true"},
					"X-Codex-Turn-State":   {"turn-state"},
				},
				Body: io.NopCloser(strings.NewReader("data: chunk\n\ndata: [DONE]\n\n")),
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			var received []string
			StreamScannerHandler(c, resp, info, func(data string, _ *StreamResult) {
				received = append(received, data)
				assert.NoError(t, StringData(c, data))
			})
			stop()

			require.NotNil(t, info.StreamStatus)
			assert.True(t, info.StreamStatus.IsNormalEnd())
			assert.Equal(t, []string{"chunk"}, received)
			assert.True(t, strings.HasSuffix(writer.Body.String(), "data: chunk\n\n"))
			if test.waitForPing {
				assert.True(t, strings.HasPrefix(writer.Body.String(), ": PING\n\n"))
			} else {
				assert.Equal(t, "data: chunk\n\n", writer.Body.String())
				assert.Equal(t, "true", writer.Result().Header.Get("X-Reasoning-Included"))
				assert.Equal(t, "turn-state", writer.Result().Header.Get("X-Codex-Turn-State"))
			}
		})
	}
}
