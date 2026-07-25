package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInitChannelMetaResetsUpstreamStreamBetweenRetries(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)

	info := &relaycommon.RelayInfo{
		IsStream:         false,
		UpstreamIsStream: true,
	}
	info.InitChannelMeta(c)

	require.False(t, info.UpstreamIsStream)
}

func TestForceUpstreamStreamScopeAndBodyOverride(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ChannelSetting: dto.ChannelSettings{
				ForceUpstreamStream: true,
			},
		},
	}
	require.True(t, info.ShouldForceUpstreamStream())

	info.ChannelType = constant.ChannelTypeAnthropic
	require.False(t, info.ShouldForceUpstreamStream())
	info.ChannelType = constant.ChannelTypeOpenAI
	info.RelayMode = relayconstant.RelayModeResponses
	require.True(t, info.ShouldForceUpstreamStream())
	info.RelayMode = relayconstant.RelayModeEmbeddings
	require.False(t, info.ShouldForceUpstreamStream())

	info.ParamOverride = map[string]any{"stream": false}
	body, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"stream":true,"custom":{"preserved":true}}`), info)
	require.NoError(t, err)
	var overridden map[string]any
	require.NoError(t, common.Unmarshal(body, &overridden))
	require.Equal(t, false, overridden["stream"])

	body, err = setUpstreamStream(body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(body, &decoded))
	require.Equal(t, true, decoded["stream"])
	require.Equal(t, map[string]any{"preserved": true}, decoded["custom"])
}
