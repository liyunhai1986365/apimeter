package helper

import (
	"context"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// StreamResult is passed to each dataHandler invocation, providing methods
// to record soft errors, signal fatal stops, or mark normal completion.
// StreamScannerHandler checks IsStopped() after each callback invocation.
type StreamResult struct {
	ctx         context.Context
	status      *relaycommon.StreamStatus
	stopped     bool
	stopReading func()
}

func newStreamResult(ctx context.Context, status *relaycommon.StreamStatus) *StreamResult {
	return &StreamResult{ctx: ctx, status: status}
}

// Context cancels blocking callback work on client cancellation or stream
// timeout. Normal upstream EOF still allows queued events to finish processing.
func (r *StreamResult) Context() context.Context { return r.ctx }

// Error records a soft error. The stream continues processing.
// Can be called multiple times per chunk.
func (r *StreamResult) Error(err error) {
	if err == nil {
		return
	}
	r.status.RecordError(err.Error())
}

// Stop records a fatal error and marks the stream to stop after this chunk.
func (r *StreamResult) Stop(err error) {
	if err != nil {
		r.status.RecordError(err.Error())
	}
	r.status.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
	r.stopped = true
}

// StopReading closes the upstream body while allowing already buffered frames
// to drain. The caller must stop downstream I/O and only collect accounting
// data on subsequent invocations. Unlike Stop, this does not discard the queue.
func (r *StreamResult) StopReading(err error) {
	if err != nil {
		r.status.RecordError(err.Error())
	}
	r.status.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
	if r.stopReading != nil {
		r.stopReading()
	}
}

// Done signals that the handler has finished processing normally
// (e.g., Dify "message_end"). The stream stops after this chunk.
func (r *StreamResult) Done() {
	r.status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	r.stopped = true
}

// IsStopped returns whether Stop() or Done() was called during this chunk.
func (r *StreamResult) IsStopped() bool {
	return r.stopped
}

// reset clears the per-chunk stopped flag so the object can be reused.
func (r *StreamResult) reset() {
	r.stopped = false
}
