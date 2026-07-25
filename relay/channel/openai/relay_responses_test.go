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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesBufferedStreamHandlerReturnsCompleteResponsesJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"buffered response"}`,
		`data: {"type":"response.content_part.done","output_index":0,"content_index":0,"part":{"type":"output_text","text":"buffered response","annotations":[{"type":"url_citation","url":"https://example.com","title":"Example","start_index":0,"end_index":8}],"logprobs":[]}}`,
		`data: {"type":"response.completed","response":{"id":"resp_buffered","object":"response","model":"gpt-5-mini","status":"completed","completed_at":1780632413,"service_tier":"default","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"buffered response"}]}],"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	info := &relaycommon.RelayInfo{
		StartTime:   time.Unix(1780632412, 0),
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5-mini"},
	}
	c, recorder, resp := newTextTestContext(t, "/v1/responses", body, "text/event-stream", info)

	usage, apiErr := OaiResponsesBufferedStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 13, usage.TotalTokens)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	var response dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "resp_buffered", response.ID)
	require.Len(t, response.Output, 1)
	require.Equal(t, "buffered response", response.Output[0].Content[0].Text)
	require.Len(t, response.Output[0].Content[0].Annotations, 1)
	require.NotNil(t, response.Output[0].Content[0].Logprobs)
	require.Empty(t, response.Output[0].Content[0].Logprobs)
	annotation, ok := response.Output[0].Content[0].Annotations[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "url_citation", annotation["type"])
	var responseJSON map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &responseJSON))
	require.Equal(t, float64(1780632413), responseJSON["completed_at"])
	require.Equal(t, "default", responseJSON["service_tier"])
}

func TestOaiResponsesStreamHandlerConvertsChatCompletionChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-dummy","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5.4-mini","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`data: {"id":"chatcmpl-dummy","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5.4-mini","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`data: {"id":"chatcmpl-dummy","object":"chat.completion.chunk","created":1780632412,"model":"gpt-5.4-mini","choices":[],"usage":{"prompt_tokens":322,"completion_tokens":95,"total_tokens":417,"prompt_tokens_details":{"cached_tokens":73088}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := &relaycommon.RelayInfo{
		StartTime: time.Unix(1780632412, 0),
		IsStream:  true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.4-mini",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	usage, err := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, &dto.Usage{
		PromptTokens:     322,
		CompletionTokens: 95,
		TotalTokens:      417,
		InputTokens:      322,
		OutputTokens:     95,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 73088,
		},
	}, usage)

	output := recorder.Body.String()
	require.NotContains(t, output, "chat.completion.chunk")
	require.NotContains(t, output, "chatcmpl-dummy")
	require.Contains(t, output, "event: response.output_text.delta")
	require.Contains(t, output, `"delta":"hello"`)
	require.Contains(t, output, "event: response.completed")
	require.Contains(t, output, `"input_tokens":322`)
	require.Contains(t, output, `"output_tokens":95`)
	require.Contains(t, output, `"cached_tokens":73088`)
}

func TestOaiResponsesStreamHandlerRecordsDynamicWebSearchCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})

	body := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call"}}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	webSearchInfo := &relaycommon.RelayInfo{
		StartTime: time.Unix(1780632412, 0),
		IsStream:  true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.4-mini",
		},
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{},
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	usage, err := OaiResponsesStreamHandler(c, webSearchInfo, resp)

	require.Nil(t, err)
	require.Equal(t, 2, usage.TotalTokens)
	webSearchTool, exists := webSearchInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearch]
	require.True(t, exists)
	require.Equal(t, 1, webSearchTool.CallCount)
}

func TestOaiResponsesHandlerRecordsOnlyActualWebSearchCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		output    string
		callCount int
	}{
		{name: "configured but unused", output: `[{"type":"message","role":"assistant","content":[]}]`},
		{name: "called", output: `[{"type":"web_search_call","id":"ws_1"}]`, callCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
					BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
						dto.BuildInToolWebSearch: {ToolName: dto.BuildInToolWebSearch},
					},
				},
			}
			body := fmt.Sprintf(`{"object":"response","tools":[{"type":"web_search"}],"output":%s,"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`, tt.output)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

			_, apiErr := OaiResponsesHandler(c, info, resp)

			require.Nil(t, apiErr)
			require.Equal(t, tt.callCount, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearch].CallCount)
		})
	}
}
