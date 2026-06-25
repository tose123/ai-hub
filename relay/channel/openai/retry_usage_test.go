package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTextTestContext(t *testing.T, path string, body string, contentType string, info *relaycommon.RelayInfo) (*gin.Context, *httptest.ResponseRecorder, *http.Response) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Set("id", 1)
	c.Set("token_name", "default")
	c.Set("group", "default")
	if info.StartTime.IsZero() {
		info.StartTime = time.Now()
	}
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{contentType}},
	}
	return c, recorder, resp
}

func TestOpenaiHandlerRetriesWhenUsageIsZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	body := `{"id":"chatcmpl-a","object":"chat.completion","created":1,"model":"gpt-5","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	c, recorder, resp := newTextTestContext(t, "/v1/chat/completions", body, "application/json", info)

	usage, err := OpenaiHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponse, err.GetErrorCode())
	require.Equal(t, http.StatusBadGateway, err.StatusCode)
	require.Empty(t, recorder.Body.String())
}

func TestOaiStreamHandlerWritesTerminalErrorAfterPartialOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		IsStream:          true,
		RelayMode:         relayconstant.RelayModeChatCompletions,
		RelayFormat:       types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5-mini",
		},
		StartTime: time.Unix(1780632412, 0),
	}
	info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-a","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`data: {"id":"chatcmpl-a","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp := newTextTestContext(t, "/v1/chat/completions", body, "text/event-stream", info)

	usage, err := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponse, err.GetErrorCode())
	output := recorder.Body.String()
	require.Contains(t, output, `data: {"id":"chatcmpl-a"`)
	require.Contains(t, output, `"content":"hello"`)
	require.Contains(t, output, `data: {"error":`)
	require.NotContains(t, output, `[DONE]`)
}

func TestOaiResponsesToChatHandlerRetriesWhenFallbackStillZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5",
		},
	}
	body := `{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"gpt-5","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`
	c, recorder, resp := newTextTestContext(t, "/v1/chat/completions", body, "application/json", info)

	usage, err := OaiResponsesToChatHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponse, err.GetErrorCode())
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesStreamHandlerWritesFailedInsteadOfCompletedWhenUsageInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		StartTime:         time.Unix(1780632412, 0),
		IsStream:          true,
		RelayMode:         relayconstant.RelayModeResponses,
		RelayFormat:       types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-3.5-turbo",
		},
	}
	info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hello","item_id":"msg_0","output_index":0,"content_index":0}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","created_at":1780632412,"status":"completed","model":"gpt-3.5-turbo","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp := newTextTestContext(t, "/v1/responses", body, "text/event-stream", info)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponse, err.GetErrorCode())
	output := recorder.Body.String()
	require.Contains(t, output, `event: response.output_text.delta`)
	require.Contains(t, output, `"delta":"hello"`)
	require.Contains(t, output, `event: response.failed`)
	require.NotContains(t, output, `event: response.completed`)
}

func TestOaiResponsesStreamHandlerRetriesWhenUsageInvalidBeforeAnyChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		StartTime:         time.Unix(1780632412, 0),
		IsStream:          true,
		RelayMode:         relayconstant.RelayModeResponses,
		RelayFormat:       types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-3.5-turbo",
		},
	}
	info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","created_at":1780632412,"status":"completed","model":"gpt-3.5-turbo","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp := newTextTestContext(t, "/v1/responses", body, "text/event-stream", info)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponse, err.GetErrorCode())
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesToChatStreamHandlerWritesTerminalErrorAfterPartialOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		StartTime:         time.Unix(1780632412, 0),
		IsStream:          true,
		RelayMode:         relayconstant.RelayModeChatCompletions,
		RelayFormat:       types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-3.5-turbo",
		},
	}
	info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hello","item_id":"msg_0","output_index":0,"content_index":0}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","created_at":1780632412,"status":"completed","model":"gpt-3.5-turbo","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp := newTextTestContext(t, "/v1/chat/completions", body, "text/event-stream", info)

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponse, err.GetErrorCode())
	output := recorder.Body.String()
	require.Contains(t, output, `data: {"id":"chatcmpl-`)
	require.Contains(t, output, `"content":"hello"`)
	require.Contains(t, output, `data: {"error":`)
	require.NotContains(t, output, `[DONE]`)
}

func TestHasValidTextUsageAcceptsInputOutputTokens(t *testing.T) {
	usage := &dto.Usage{InputTokens: 10, OutputTokens: 2}
	require.True(t, hasValidTextUsage(usage))
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 12, usage.TotalTokens)
}
