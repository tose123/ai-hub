package model_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseModelForMatching(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "xhigh effort", model: "claude-opus-4-7-xhigh", want: "claude-opus-4-7"},
		{name: "none effort", model: "gpt-5-none", want: "gpt-5"},
		{name: "thinking budget", model: "gemini-2.5-flash-thinking-1024", want: "gemini-2.5-flash"},
		{name: "preserved thinking model", model: "kimi-k2-thinking", want: "kimi-k2-thinking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BaseModelForMatching(tt.model); got != tt.want {
				t.Fatalf("BaseModelForMatching(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestGlobalModelMappingCanBeCleared(t *testing.T) {
	originalMapping := globalSettings.ModelMapping
	globalSettings.ModelMapping = map[string]string{
		"alias-model": "upstream-model",
	}
	t.Cleanup(func() {
		globalSettings.ModelMapping = originalMapping
	})

	require.NoError(t, config.UpdateConfigFromMap(&globalSettings, map[string]string{
		"model_mapping": "{}",
	}))
	assert.Empty(t, globalSettings.ModelMapping)
}
