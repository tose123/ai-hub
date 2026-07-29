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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
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
	body := `{"id":"chatcmpl-a","object":"chat.completion","created":1,"model":"gpt-5","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":0,"total_tokens":5}}`
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
	require.Equal(t, http.StatusInternalServerError, err.StatusCode)
	require.Contains(t, err.Error(), "上游没有返回计费信息")
	require.NotContains(t, err.Error(), "Retryable error")
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
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
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
	require.Contains(t, output, "Retryable error")
	require.NotContains(t, output, `[DONE]`)
}

func TestOaiStreamHandlerBuffersRoleOnlyBeforeInvalidUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5-mini"},
		StartTime:   time.Unix(1780632412, 0),
	}
	info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-a","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`data: {"id":"chatcmpl-a","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5-mini","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":0,"total_tokens":5}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp := newTextTestContext(t, "/v1/chat/completions", body, "text/event-stream", info)

	usage, err := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "上游没有返回计费信息")
	require.NotContains(t, err.Error(), "Retryable error")
	require.Empty(t, recorder.Body.String())
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
		StartTime:   time.Unix(1780632412, 0),
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeResponses,
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-3.5-turbo",
		},
	}
	info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","object":"response","created_at":1780632412,"status":"in_progress","model":"gpt-3.5-turbo"}}`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1","object":"response","created_at":1780632412,"status":"in_progress","model":"gpt-3.5-turbo"}}`,
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
	require.Contains(t, output, `event: response.created`)
	require.Contains(t, output, `event: response.output_text.delta`)
	require.Contains(t, output, `"delta":"hello"`)
	require.Contains(t, output, `event: response.failed`)
	require.Contains(t, output, "Retryable error")
	require.NotContains(t, output, `event: response.completed`)
	require.Less(t, strings.Index(output, `event: response.created`), strings.Index(output, `event: response.output_text.delta`))
}

func TestOaiResponsesStreamHandlerRetriesWhenUsageInvalidBeforeAnyChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		StartTime:   time.Unix(1780632412, 0),
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeResponses,
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-3.5-turbo",
		},
	}
	info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","object":"response","created_at":1780632412,"status":"in_progress","model":"gpt-3.5-turbo"}}`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1","object":"response","created_at":1780632412,"status":"in_progress","model":"gpt-3.5-turbo"}}`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_0","status":"in_progress","role":"assistant","content":[]}}`,
		`data: {"type":"response.content_part.added","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","created_at":1780632412,"status":"completed","model":"gpt-3.5-turbo","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp := newTextTestContext(t, "/v1/responses", body, "text/event-stream", info)

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeBadResponse, err.GetErrorCode())
	require.Contains(t, err.Error(), "上游没有返回计费信息")
	require.NotContains(t, err.Error(), "Retryable error")
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesToChatStreamHandlerWritesTerminalErrorAfterPartialOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	info := &relaycommon.RelayInfo{
		StartTime:   time.Unix(1780632412, 0),
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
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
	require.Contains(t, output, "Retryable error")
	require.NotContains(t, output, `[DONE]`)
}

func TestTextHandlersRejectZeroOutputUsageBeforeWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	tests := []struct {
		name string
		path string
		body string
		ct   string
		run  func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{
			name: "responses compact",
			path: "/v1/responses/compact",
			body: `{"id":"resp_1","object":"response.compaction","output":[],"usage":{"input_tokens":5,"output_tokens":0,"total_tokens":5}}`,
			ct:   "application/json",
			run: func(c *gin.Context, _ *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
				return OaiResponsesCompactionHandler(c, resp)
			},
		},
		{
			name: "responses via chat",
			path: "/v1/responses",
			body: `{"id":"chatcmpl-a","object":"chat.completion","created":1,"model":"gpt-5","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":0,"total_tokens":5}}`,
			ct:   "application/json",
			run:  OaiChatToResponsesHandler,
		},
		{
			name: "forced upstream chat stream",
			path: "/v1/chat/completions",
			body: strings.Join([]string{
				`data: {"id":"chatcmpl-a","object":"chat.completion.chunk","created":1,"model":"gpt-5","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
				`data: {"id":"chatcmpl-a","object":"chat.completion.chunk","created":1,"model":"gpt-5","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":0,"total_tokens":5}}`,
				`data: [DONE]`,
				``,
			}, "\n"),
			ct:  "text/event-stream",
			run: OaiBufferedStreamHandler,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RelayMode:   relayconstant.RelayModeChatCompletions,
				RelayFormat: types.RelayFormatOpenAI,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5"},
				StartTime:   time.Unix(1780632412, 0),
			}
			if strings.HasPrefix(tt.path, "/v1/responses") {
				info.RelayMode = relayconstant.RelayModeResponses
			}
			info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
			c, recorder, resp := newTextTestContext(t, tt.path, tt.body, tt.ct, info)

			usage, err := tt.run(c, info, resp)

			require.Nil(t, usage)
			require.NotNil(t, err)
			require.Contains(t, err.Error(), "上游没有返回计费信息")
			require.NotContains(t, err.Error(), "Retryable error")
			require.Empty(t, recorder.Body.String())
		})
	}
}

func TestResponsesTerminalReasonsSkipRetryBeforeOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	tests := []struct {
		name  string
		event string
	}{
		{name: "invalid request", event: `{"type":"response.failed","response":{"status":"failed","error":{"message":"bad request","type":"invalid_request_error","code":"invalid_request"}}}`},
		{name: "content policy refusal", event: `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"cannot comply"}]}],"usage":{"input_tokens":5,"output_tokens":0,"total_tokens":5}}}`},
		{name: "content filter", event: `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`},
		{name: "max output tokens", event: `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				IsStream:    true,
				RelayMode:   relayconstant.RelayModeResponses,
				RelayFormat: types.RelayFormatOpenAIResponses,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5"},
				StartTime:   time.Unix(1780632412, 0),
			}
			info.UpstreamModelName = info.ChannelMeta.UpstreamModelName
			body := "data: " + tt.event + "\n\ndata: [DONE]\n\n"
			c, recorder, resp := newTextTestContext(t, "/v1/responses", body, "text/event-stream", info)

			usage, err := OaiResponsesStreamHandler(c, info, resp)

			require.Nil(t, usage)
			require.NotNil(t, err)
			require.True(t, types.IsSkipRetryError(err))
			require.Empty(t, recorder.Body.String())
		})
	}
}

func TestHasValidTextUsageAcceptsInputOutputTokens(t *testing.T) {
	usage := &dto.Usage{InputTokens: 10, OutputTokens: 2}
	require.True(t, hasValidTextUsage(usage))
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 12, usage.TotalTokens)
}
