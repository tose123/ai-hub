package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeNotImplementedError(t *testing.T) {
	err := NewErrorWithStatusCode(
		errors.New("status_code=500, not implemented"),
		ErrorCodeDoRequestFailed,
		http.StatusInternalServerError,
	)

	require.True(t, IsNotImplementedError(err))
	require.Equal(t, ErrorCodeAPINotImplemented, err.GetErrorCode())
	require.Equal(t, http.StatusNotFound, err.StatusCode)
	require.True(t, IsSkipRetryError(err))
}
