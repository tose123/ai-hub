package channel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	commonroot "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func configureRelayHTTPClientForTest(t *testing.T, insecureTLS bool) {
	t.Helper()

	oldRelayTimeout := commonroot.RelayTimeout
	oldHeaderTimeout := commonroot.RelayResponseHeaderTimeout
	oldDialTimeout := commonroot.RelayDialTimeout
	oldTLSHandshakeTimeout := commonroot.RelayTLSHandshakeTimeout
	oldMaxIdleConns := commonroot.RelayMaxIdleConns
	oldMaxIdleConnsPerHost := commonroot.RelayMaxIdleConnsPerHost
	oldTLSInsecureSkipVerify := commonroot.TLSInsecureSkipVerify

	commonroot.RelayTimeout = 0
	commonroot.RelayResponseHeaderTimeout = 1
	commonroot.RelayDialTimeout = 1
	commonroot.RelayTLSHandshakeTimeout = 1
	commonroot.RelayMaxIdleConns = 10
	commonroot.RelayMaxIdleConnsPerHost = 10
	commonroot.TLSInsecureSkipVerify = insecureTLS
	service.InitHttpClient()

	t.Cleanup(func() {
		commonroot.RelayTimeout = oldRelayTimeout
		commonroot.RelayResponseHeaderTimeout = oldHeaderTimeout
		commonroot.RelayDialTimeout = oldDialTimeout
		commonroot.RelayTLSHandshakeTimeout = oldTLSHandshakeTimeout
		commonroot.RelayMaxIdleConns = oldMaxIdleConns
		commonroot.RelayMaxIdleConnsPerHost = oldMaxIdleConnsPerHost
		commonroot.TLSInsecureSkipVerify = oldTLSInsecureSkipVerify
		service.InitHttpClient()
	})
}

func relayInfoForRequestTest(modelName string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: modelName,
		},
	}
}

func TestDoRequest_ClientCanceledBeforeUpstreamRequest(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	reqCtx, cancel := context.WithCancel(req.Context())
	cancel()
	ctx.Request = req.WithContext(reqCtx)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	resp, err := doRequest(ctx, upstreamReq, relayInfoForRequestTest(""))

	require.Nil(t, resp)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, types.ErrorCodeClientCanceled, apiErr.GetErrorCode())
	require.Equal(t, types.StatusClientClosedRequest, apiErr.StatusCode)
	require.True(t, types.IsSkipRetryError(apiErr))
	require.True(t, types.IsClientCanceledError(apiErr))
	require.True(t, errors.Is(apiErr, context.Canceled))
}

func TestDoRequest_ResponseHeaderTimeoutCancelsHTTP1UpstreamRequest(t *testing.T) {
	configureRelayHTTPClientForTest(t, false)
	gin.SetMode(gin.TestMode)

	started := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(3 * time.Second):
			t.Error("upstream request context was not canceled")
		}
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	upstreamReq, reqErr := http.NewRequest(http.MethodPost, server.URL, nil)
	require.NoError(t, reqErr)
	resp, err := doRequest(ctx, upstreamReq, relayInfoForRequestTest(""))

	require.Nil(t, resp)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, types.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
	require.False(t, types.IsSkipRetryError(apiErr))
	require.False(t, types.IsClientCanceledError(apiErr))
	require.Equal(t, "upstream error: response header timeout", apiErr.Error())

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream server did not receive request")
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request context was not canceled after response header timeout")
	}
}

func TestDoRequest_ResponseHeaderTimeoutCancelsHTTP2UpstreamStream(t *testing.T) {
	configureRelayHTTPClientForTest(t, true)
	gin.SetMode(gin.TestMode)

	started := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, 2, r.ProtoMajor)
		close(started)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(3 * time.Second):
			t.Error("upstream HTTP/2 stream context was not canceled")
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	upstreamReq, reqErr := http.NewRequest(http.MethodPost, server.URL, nil)
	require.NoError(t, reqErr)
	resp, err := doRequest(ctx, upstreamReq, relayInfoForRequestTest(""))

	require.Nil(t, resp)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, types.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
	require.False(t, types.IsSkipRetryError(apiErr))
	require.Equal(t, "upstream error: response header timeout", apiErr.Error())

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream server did not receive request")
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream HTTP/2 stream was not canceled after response header timeout")
	}
}

func TestDoRequest_ResponseHeaderTimeoutStopsAfterHeaders(t *testing.T) {
	configureRelayHTTPClientForTest(t, false)
	gin.SetMode(gin.TestMode)

	allowFinish := make(chan struct{})
	canceledBeforeFinish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-allowFinish:
			_, _ = w.Write([]byte("ok"))
		case <-r.Context().Done():
			close(canceledBeforeFinish)
		}
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	upstreamReq, reqErr := http.NewRequest(http.MethodPost, server.URL, nil)
	require.NoError(t, reqErr)
	resp, err := doRequest(ctx, upstreamReq, relayInfoForRequestTest(""))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	time.Sleep(1200 * time.Millisecond)
	select {
	case <-canceledBeforeFinish:
		t.Fatal("header timeout canceled request after response headers were received")
	default:
	}

	close(allowFinish)
	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	require.Equal(t, "ok", string(body))
}

func TestDoRequest_ClientCanceledAfterUpstreamRequestStillReturnsResponse(t *testing.T) {
	configureRelayHTTPClientForTest(t, false)
	gin.SetMode(gin.TestMode)

	started := make(chan struct{})
	allowFinish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-allowFinish
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"usage":{"total_tokens":1}}`))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	reqCtx, cancelClient := context.WithCancel(req.Context())
	ctx.Request = req.WithContext(reqCtx)

	upstreamReq, reqErr := http.NewRequest(http.MethodPost, server.URL, nil)
	require.NoError(t, reqErr)

	resultCh := make(chan struct {
		resp *http.Response
		err  error
	}, 1)
	go func() {
		resp, err := doRequest(ctx, upstreamReq, relayInfoForRequestTest(""))
		resultCh <- struct {
			resp *http.Response
			err  error
		}{resp: resp, err: err}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream server did not receive request")
	}
	cancelClient()
	close(allowFinish)

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		require.NotNil(t, result.resp)
		defer result.resp.Body.Close()
		body, readErr := io.ReadAll(result.resp.Body)
		require.NoError(t, readErr)
		require.Equal(t, `{"usage":{"total_tokens":1}}`, string(body))
		require.True(t, commonroot.IsClientGone(ctx))
	case <-time.After(2 * time.Second):
		t.Fatal("doRequest did not finish after upstream response")
	}
}

func TestDoRequest_DoesNotMutateSharedTransportResponseHeaderTimeout(t *testing.T) {
	configureRelayHTTPClientForTest(t, false)
	gin.SetMode(gin.TestMode)

	client := service.GetHttpClient()
	require.NotNil(t, client)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Zero(t, transport.ResponseHeaderTimeout)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	upstreamReq, reqErr := http.NewRequest(http.MethodPost, server.URL+"/v1/images/generations", nil)
	require.NoError(t, reqErr)
	resp, err := doRequest(ctx, upstreamReq, relayInfoForRequestTest("gpt-image-1"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	require.Zero(t, transport.ResponseHeaderTimeout)
}

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}
