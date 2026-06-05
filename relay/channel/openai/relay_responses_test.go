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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
