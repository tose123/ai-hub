package service

import (
	"math"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const defaultCachedTokensRatio = 1.0

func channelCachedTokensRatio(relayInfo *relaycommon.RelayInfo) (float64, bool) {
	if relayInfo == nil || relayInfo.ChannelMeta == nil || relayInfo.ChannelSetting.CachedTokensRatio == nil {
		return defaultCachedTokensRatio, false
	}

	ratio := *relayInfo.ChannelSetting.CachedTokensRatio
	if ratio < 0 || ratio > 1 {
		return defaultCachedTokensRatio, false
	}
	return ratio, true
}

func scaleTokenCount(tokens int, ratio float64) int {
	if tokens <= 0 {
		return 0
	}
	return int(math.Round(float64(tokens) * ratio))
}

func ApplyChannelCachedTokensRatioToUsage(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) {
	if usage == nil {
		return
	}

	ratio, ok := channelCachedTokensRatio(relayInfo)
	if !ok || ratio == defaultCachedTokensRatio {
		return
	}

	usage.PromptTokensDetails.CachedTokens = scaleTokenCount(usage.PromptTokensDetails.CachedTokens, ratio)
	if usage.PromptCacheHitTokens > 0 {
		usage.PromptCacheHitTokens = scaleTokenCount(usage.PromptCacheHitTokens, ratio)
	}
	if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
		usage.InputTokensDetails.CachedTokens = scaleTokenCount(usage.InputTokensDetails.CachedTokens, ratio)
	}

	usage.PromptTokensDetails.CachedCreationTokens = scaleTokenCount(usage.PromptTokensDetails.CachedCreationTokens, ratio)
	usage.ClaudeCacheCreation5mTokens = scaleTokenCount(usage.ClaudeCacheCreation5mTokens, ratio)
	usage.ClaudeCacheCreation1hTokens = scaleTokenCount(usage.ClaudeCacheCreation1hTokens, ratio)

	relayInfo.ChannelSetting.CachedTokensRatio = nil
}
