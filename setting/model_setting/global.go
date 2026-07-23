package model_setting

import (
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids,omitempty"`
	ChannelTypes  []int    `json:"channel_types,omitempty"`
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}

	if channelID > 0 && len(p.ChannelIDs) > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && len(p.ChannelTypes) > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

type GlobalSettings struct {
	PassThroughRequestEnabled        bool                             `json:"pass_through_request_enabled"`
	ModelMapping                     map[string]string                `json:"model_mapping"`
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`
}

// 默认配置
var defaultOpenaiSettings = GlobalSettings{
	PassThroughRequestEnabled: false,
	ModelMapping:              map[string]string{},
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
	ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
}

// 全局实例
var globalSettings = defaultOpenaiSettings

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("global", &globalSettings)
}

func GetGlobalSettings() *GlobalSettings {
	return &globalSettings
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking/-low/-high/-medium 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	for _, entry := range globalSettings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}

var matchingEffortSuffixes = []string{"-max", "-xhigh", "-high", "-medium", "-low", "-minimal", "-none"}

const compactModelSuffixForMatching = "-openai-compact"

func BaseModelForMatching(modelName string) string {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return target
	}

	compactSuffix := ""
	if strings.HasSuffix(target, compactModelSuffixForMatching) {
		target = strings.TrimSuffix(target, compactModelSuffixForMatching)
		compactSuffix = compactModelSuffixForMatching
	}

	if !ShouldPreserveThinkingSuffix(target) {
		if base, ok := trimThinkingSuffixForMatching(target); ok {
			return base + compactSuffix
		}
	}

	for _, suffix := range matchingEffortSuffixes {
		if strings.HasSuffix(target, suffix) && len(target) > len(suffix) {
			return strings.TrimSuffix(target, suffix) + compactSuffix
		}
	}

	return target + compactSuffix
}

func trimThinkingSuffixForMatching(modelName string) (string, bool) {
	for _, suffix := range []string{"-nothinking", "-thinking"} {
		if strings.HasSuffix(modelName, suffix) && len(modelName) > len(suffix) {
			return strings.TrimSuffix(modelName, suffix), true
		}
	}

	thinkingBudgetSep := "-thinking-"
	idx := strings.LastIndex(modelName, thinkingBudgetSep)
	if idx <= 0 {
		return "", false
	}
	budget := modelName[idx+len(thinkingBudgetSep):]
	if budget == "" {
		return "", false
	}
	if _, err := strconv.Atoi(budget); err != nil {
		return "", false
	}
	return modelName[:idx], true
}
