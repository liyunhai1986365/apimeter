package common

import (
	"errors"
	"sync"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// StreamUsageError carries the usage snapshot from an interrupted stream.
// The controller settles it before error handling/refunds, independently of
// the client protocol or any intermediate request/response conversion.
type StreamUsageError struct {
	cause error
	usage *dto.Usage
	once  sync.Once
}

func (e *StreamUsageError) Error() string { return e.cause.Error() }
func (e *StreamUsageError) Unwrap() error { return e.cause }
func (e *StreamUsageError) SettleOnce(settle func(*dto.Usage)) {
	e.once.Do(func() { settle(e.usage) })
}

// WithStreamUsage is only called for failures after upstream streaming began.
// Ordinary request/upstream rejection errors keep their existing refund policy.
// Keep zero-token snapshots: settlement also considers cache and tool charges.
func WithStreamUsage(apiErr *types.NewAPIError, usage *dto.Usage) *types.NewAPIError {
	if apiErr == nil {
		return nil
	}
	types.ErrOptionWithSkipRetry()(apiErr)
	if usage == nil {
		return apiErr
	}
	var existing *StreamUsageError
	if errors.As(apiErr, &existing) {
		return apiErr
	}
	cause := apiErr.Err
	if cause == nil {
		cause = errors.New(apiErr.Error())
	}
	apiErr.Err = &StreamUsageError{cause: cause, usage: usage}
	return apiErr
}
