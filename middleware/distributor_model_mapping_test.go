package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveRoutedModelAppliesGlobalBeforeTokenMapping(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	originalMapping := settings.ModelMapping
	settings.ModelMapping = map[string]string{
		"alias-model": "upstream-model",
	}
	t.Cleanup(func() {
		settings.ModelMapping = originalMapping
	})

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelMapping, `{
		"alias-model": "token-only-model",
		"upstream-model": "final-model"
	}`)

	routedModel, baseModel, mapped, err := resolveRoutedModel(ctx, "alias-model")

	require.NoError(t, err)
	require.Equal(t, "final-model", routedModel)
	require.Equal(t, "final-model", baseModel)
	require.True(t, mapped)
}

func TestResolveRoutedModelKeepsMappedFlagWhenFinalNameMatchesRequest(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	originalMapping := settings.ModelMapping
	settings.ModelMapping = map[string]string{"model-a": "model-b"}
	t.Cleanup(func() {
		settings.ModelMapping = originalMapping
	})

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelMapping, `{"model-b":"model-a"}`)

	routedModel, _, mapped, err := resolveRoutedModel(ctx, "model-a")

	require.NoError(t, err)
	require.Equal(t, "model-a", routedModel)
	require.True(t, mapped)
}

func TestApplyModelMappingFollowsChain(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	mappedModel, mapped, err := applyModelMapping(ctx, "model-a", map[string]string{
		"model-a": "model-b",
		"model-b": "model-c",
	})

	require.NoError(t, err)
	require.Equal(t, "model-c", mappedModel)
	require.True(t, mapped)
}

func TestGetModelRequestPreservesCompactClientModel(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	originalMapping := settings.ModelMapping
	settings.ModelMapping = map[string]string{"model-a": "model-b"}
	t.Cleanup(func() {
		settings.ModelMapping = originalMapping
	})

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(`{"model":"model-a"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	request, _, err := getModelRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "model-a", common.GetContextKeyString(ctx, constant.ContextKeyRequestModel))

	routedModel, _, mapped, err := resolveRoutedModel(ctx, request.Model)
	require.NoError(t, err)
	require.True(t, mapped)
	require.Equal(t, "model-b", strings.TrimSuffix(routedModel, ratio_setting.CompactModelSuffix))
}
