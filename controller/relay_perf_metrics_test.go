package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
)

func TestShouldRecordPerfFailure(t *testing.T) {
	tests := []struct {
		name       string
		err        *types.NewAPIError
		wantRecord bool
	}{
		{
			name:       "nil error is not recorded",
			err:        nil,
			wantRecord: false,
		},
		{
			name: "bad request is not recorded",
			err: &types.NewAPIError{
				StatusCode: http.StatusBadRequest,
			},
			wantRecord: false,
		},
		{
			name: "unprocessable entity is not recorded",
			err: &types.NewAPIError{
				StatusCode: http.StatusUnprocessableEntity,
			},
			wantRecord: false,
		},
		{
			name: "rate limit is recorded",
			err: &types.NewAPIError{
				StatusCode: http.StatusTooManyRequests,
			},
			wantRecord: true,
		},
		{
			name: "server error is recorded",
			err: &types.NewAPIError{
				StatusCode: http.StatusInternalServerError,
			},
			wantRecord: true,
		},
		{
			name:       "unknown status is recorded",
			err:        &types.NewAPIError{},
			wantRecord: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRecordPerfFailure(tt.err); got != tt.wantRecord {
				t.Fatalf("shouldRecordPerfFailure() = %v, want %v", got, tt.wantRecord)
			}
		})
	}
}
