package relay

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestNewImageConversionErrorMarksInvalidRequestAsNonRetryable(t *testing.T) {
	apiErr := newImageConversionError(types.MarkRequestError(errors.New("image is required")))

	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.True(t, types.IsSkipRetryError(apiErr))
}

func TestNewImageConversionErrorKeepsTransientFailureRetryable(t *testing.T) {
	apiErr := newImageConversionError(errors.New("download image failed: timeout"))

	require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	require.False(t, types.IsSkipRetryError(apiErr))
}
