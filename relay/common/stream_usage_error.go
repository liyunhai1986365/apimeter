package common

import (
	"errors"
	"sync"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// StreamUsageError carries usage already incurred before a stream failed.
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
func WithStreamUsage(apiErr *types.NewAPIError, usage *dto.Usage) *types.NewAPIError {
	if apiErr == nil {
		return nil
	}
	types.ErrOptionWithSkipRetry()(apiErr)
	if !hasStreamUsage(usage) {
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

func hasStreamUsage(usage *dto.Usage) bool {
	if usage == nil {
		return false
	}
	if usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0 ||
		usage.PromptTokensDetails.CachedTokens > 0 || usage.PromptTokensDetails.CacheCreationTokensTotal() > 0 ||
		usage.ClaudeCacheCreation5mTokens > 0 || usage.ClaudeCacheCreation1hTokens > 0 {
		return true
	}
	// A converted response may expose no ordinary tokens while its canonical
	// Anthropic snapshot still contains billable cache reads or writes.
	return usage.BillingUsage != nil && dto.HasClaudeUsageTokens(usage.BillingUsage.ClaudeUsage)
}
