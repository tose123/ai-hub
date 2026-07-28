package common_handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRerankHandlerCapturesSearchUnitsAndSynthesizesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenRouter,
		},
		RelayFormat: relaytypes.RelayFormatRerank,
		PriceData: hosttypes.PriceData{
			UsePrice:   true,
			ModelPrice: 0.002857,
		},
	}
	info.SetEstimatePromptTokens(123)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(bytes.NewBufferString(`{
			"id":"gen-rerank-test",
			"model":"rerank-v4.0-pro",
			"results":[{"index":0,"relevance_score":0.9}],
			"usage":{"search_units":2,"cost":0.005},
			"provider":"Cohere"
		}`)),
	}

	usage, apiErr := RerankHandler(ctx, info, resp)
	require.Nil(t, apiErr)
	require.Equal(t, 123, usage.PromptTokens)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Equal(t, 123, usage.TotalTokens)
	require.Equal(t, 2, common.GetContextKeyInt(ctx, constant.ContextKeyRerankSearchUnits))

	var rendered dto.RerankResponse
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &rendered))
	require.Equal(t, 123, rendered.Usage.PromptTokens)
	require.Equal(t, 123, rendered.Usage.TotalTokens)
	require.Len(t, rendered.Results, 1)
}

func TestRerankHandlerFixedPriceFallbackSynthesizesUsageWhenEstimateMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenRouter,
		},
		RelayFormat: relaytypes.RelayFormatRerank,
		PriceData: hosttypes.PriceData{
			UsePrice:   true,
			ModelPrice: 0.002857,
		},
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(bytes.NewBufferString(`{
			"results":[{"index":0,"relevance_score":0.9}],
			"usage":{"cost":0.0025}
		}`)),
	}

	usage, apiErr := RerankHandler(ctx, info, resp)
	require.Nil(t, apiErr)
	require.Equal(t, 1, usage.PromptTokens)
	require.Equal(t, 1, usage.TotalTokens)
	require.Equal(t, 0, common.GetContextKeyInt(ctx, constant.ContextKeyRerankSearchUnits))
}
