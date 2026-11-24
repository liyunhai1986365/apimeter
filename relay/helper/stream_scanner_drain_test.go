package helper

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

type bufferedDrainReader struct {
	reader          *strings.Reader
	reads           atomic.Int64
	closed          atomic.Bool
	readsAfterClose atomic.Int64
}

func (r *bufferedDrainReader) Read(p []byte) (int, error) {
	r.reads.Add(1)
	if r.closed.Load() {
		r.readsAfterClose.Add(1)
	}
	return r.reader.Read(p)
}
func (r *bufferedDrainReader) Close() error { r.closed.Store(true); return nil }

func TestStreamScannerBufferedDrainPreservesTimeout(t *testing.T) {
	c, resp, info := setupStreamTest(t, strings.NewReader(""))
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	info.DisablePing = true
	body := &bufferedDrainReader{reader: strings.NewReader(strings.Repeat("data: accounting\n\n", 30))}
	resp.Body = body
	count := 0
	completed := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count++
			if count == 1 {
				<-sr.Context().Done()
			}
		}, StreamScannerOptions{DrainBufferedOnCancel: true})
		close(completed)
	}()
	select {
	case <-completed:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout cleanup deadlocked")
	}
	require.Equal(t, 30, count)
	require.Equal(t, 30, info.ReceivedResponseCount)
	require.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason)
	require.True(t, body.closed.Load())
	require.Equal(t, int64(1), body.reads.Load())
	require.Zero(t, body.readsAfterClose.Load())
}

type queueFillingDrainReader struct {
	reads  atomic.Int64
	full   chan struct{}
	closed atomic.Bool
}

func (r *queueFillingDrainReader) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.EOF
	}
	read := r.reads.Add(1)
	if read == 12 {
		close(r.full)
	}
	if read > 30 {
		return 0, io.EOF
	}
	return copy(p, "data: accounting\n\n"), nil
}
func (r *queueFillingDrainReader) Close() error { r.closed.Store(true); return nil }

func TestStreamScannerBufferedDrainReleasesStoppedHandler(t *testing.T) {
	for _, mode := range []string{"stop", "done"} {
		t.Run(mode, func(t *testing.T) {
			c, resp, info := setupStreamTest(t, strings.NewReader(""))
			info.DisablePing = true
			body := &queueFillingDrainReader{full: make(chan struct{})}
			resp.Body = body
			count := 0
			completed := make(chan struct{})
			go func() {
				StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
					count++
					<-body.full
					if mode == "stop" {
						sr.Stop(fmt.Errorf("expected stop"))
					} else {
						sr.Done()
					}
				}, StreamScannerOptions{DrainBufferedOnCancel: true})
				close(completed)
			}()
			select {
			case <-completed:
			case <-time.After(2 * time.Second):
				t.Fatal("fatal handler deadlocked with full queue")
			}
			require.Equal(t, 1, count)
			require.True(t, body.closed.Load())
			require.Equal(t, int64(12), body.reads.Load())
		})
	}
}
