package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIOCopyBytesGracefullySkipsWriteWhenClientCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	reqCtx, cancelClient := context.WithCancel(req.Context())
	c.Request = req.WithContext(reqCtx)
	cancelClient()

	resp := &http.Response{
		StatusCode: http.StatusCreated,
		Header: http.Header{
			"X-Upstream-Test": []string{"should-not-copy"},
		},
	}

	IOCopyBytesGracefully(c, resp, []byte("response body"))

	require.True(t, common.IsClientGone(c))
	require.Empty(t, recorder.Body.String())
	require.Empty(t, recorder.Header().Get("X-Upstream-Test"))
}
