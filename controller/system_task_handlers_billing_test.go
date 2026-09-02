package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestPreviousClosedBillingMonthUsesUTCMonthBoundary(t *testing.T) {
	assert.Equal(t, "2026-08", previousClosedBillingMonth(
		time.Date(2026, 9, 1, 0, 0, 15, 0, time.UTC),
	))
	assert.Equal(t, "2026-07", previousClosedBillingMonth(
		time.Date(2026, 9, 1, 7, 59, 59, 0, time.FixedZone("CST", 8*60*60)),
	))
}

func TestMonthlyBillingGenerationDueOncePerClosedMonth(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 15, 0, time.UTC)
	assert.True(t, monthlyBillingGenerationDue(now, nil))

	latestSucceeded := &model.SystemTask{
		Status:  model.SystemTaskStatusSucceeded,
		Payload: `{"month":"2026-08"}`,
	}
	assert.False(t, monthlyBillingGenerationDue(now, latestSucceeded))

	latestFailed := &model.SystemTask{
		Status:  model.SystemTaskStatusFailed,
		Payload: `{"month":"2026-08"}`,
	}
	assert.True(t, monthlyBillingGenerationDue(now, latestFailed))

	previousMonth := &model.SystemTask{
		Status:  model.SystemTaskStatusSucceeded,
		Payload: `{"month":"2026-07"}`,
	}
	assert.True(t, monthlyBillingGenerationDue(now, previousMonth))
}
