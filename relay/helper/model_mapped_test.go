package helper_test

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelMappedHelperMapsBaseModelAndPreservesEffortSuffix(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("model_mapping", `{"gpt-5.5":"gpt-5.4"}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5-xhigh",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5-xhigh",
		},
	}
	request := &dto.GeneralOpenAIRequest{
		Model:           "gpt-5.5-xhigh",
		ReasoningEffort: "low",
	}

	require.NoError(t, helper.ModelMappedHelper(ctx, info, request))
	require.Equal(t, "gpt-5.4-xhigh", info.UpstreamModelName)

	_, err := (&openai.Adaptor{}).ConvertOpenAIRequest(ctx, info, request)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", request.Model)
	require.Equal(t, "xhigh", request.ReasoningEffort)
}
