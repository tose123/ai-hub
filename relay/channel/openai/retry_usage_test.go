package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
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

func TestOaiBufferedStreamHandlerReturnsCompleteChatJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeChatCompletions,
		RelayFormat:      types.RelayFormatOpenAI,
		UpstreamIsStream: true,
		ChannelMeta:      &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5-mini"},
		StartTime:        time.Unix(1780632412, 0),
	}
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-buffered","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","system_fingerprint":"fp-buffered","service_tier":"default","choices":[{"index":0,"delta":{"role":"assistant","content":"hello "}}]}`,
		`data: {"id":"chatcmpl-buffered","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","choices":[{"index":0,"delta":{"content":"world","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},"logprobs":{"content":[{"token":"world","logprob":-0.1,"bytes":[119],"top_logprobs":[]}]},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"chatcmpl-buffered","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp := newTextTestContext(t, "/v1/chat/completions", body, "text/event-stream", info)

	usage, apiErr := OaiBufferedStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 19, usage.TotalTokens)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	var response dto.OpenAITextResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "chat.completion", response.Object)
	require.Equal(t, "fp-buffered", *response.SystemFingerprint)
	require.Equal(t, "default", common.JsonRawMessageToString(response.ServiceTier))
	require.Equal(t, "hello world", response.Choices[0].Message.StringContent())
	require.NotNil(t, response.Choices[0].Message.Annotations)
	require.Empty(t, response.Choices[0].Message.Annotations)
	require.Equal(t, "tool_calls", response.Choices[0].FinishReason)
	require.NotNil(t, response.Choices[0].Logprobs)
	logprobs, ok := (*response.Choices[0].Logprobs).(map[string]interface{})
	require.True(t, ok)
	require.Len(t, logprobs["content"], 1)
	toolCalls := response.Choices[0].Message.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	require.Equal(t, "lookup", toolCalls[0].Function.Name)
	require.JSONEq(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
}

func TestOaiBufferedStreamHandlerPreservesChatRefusal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5-mini"},
		StartTime:   time.Unix(1780632412, 0),
	}
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-refusal","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","choices":[{"index":0,"delta":{"role":"assistant","refusal":"I cannot help with that."},"logprobs":{"content":[],"refusal":[]}}]}`,
		`data: {"id":"chatcmpl-refusal","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp := newTextTestContext(t, "/v1/chat/completions", body, "text/event-stream", info)

	_, apiErr := OaiBufferedStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	var response dto.OpenAITextResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Nil(t, response.Choices[0].Message.Content)
	require.Equal(t, "I cannot help with that.", *response.Choices[0].Message.Refusal)
	require.NotNil(t, response.Choices[0].Logprobs)
	logprobs, ok := (*response.Choices[0].Logprobs).(map[string]interface{})
	require.True(t, ok)
	contentLogprobs, ok := logprobs["content"].([]interface{})
	require.True(t, ok)
	require.Empty(t, contentLogprobs)
	refusalLogprobs, ok := logprobs["refusal"].([]interface{})
	require.True(t, ok)
	require.Empty(t, refusalLogprobs)
}

func TestOpenaiHandlerRetriesWhenUsageIsZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	body := `{"id":"chatcmpl-a","object":"chat.completion","created":1,"model":"gpt-5","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	c, recorder, resp := newTextTestContext(t, "/v1/chat/completions", body, "application/json", info)
	cache := service.GetChannelAffinityCacheForTest()
	cacheKeySuffix := fmt.Sprintf("zero-token-test:%d", time.Now().UnixNano())
	cacheKey := cache.FullKey(cacheKeySuffix)
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9527, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})
	c.Set("channel_affinity_cache_key", cacheKey)
	c.Set("channel_affinity_ttl_seconds", 60)
	c.Set("channel_affinity_skip_retry_on_failure", true)

	usage, err := OpenaiHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponse, err.GetErrorCode())
	require.Equal(t, http.StatusTooManyRequests, err.StatusCode)
	require.Empty(t, recorder.Body.String())
	_, found, cacheErr := cache.Get(cacheKeySuffix)
	require.NoError(t, cacheErr)
	require.False(t, found)
	require.False(t, service.ShouldSkipRetryAfterChannelAffinityFailure(c))
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
