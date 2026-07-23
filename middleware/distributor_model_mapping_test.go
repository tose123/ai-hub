package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
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

	routedModel, baseModel, err := resolveRoutedModel(ctx, "alias-model")

	require.NoError(t, err)
	require.Equal(t, "final-model", routedModel)
	require.Equal(t, "final-model", baseModel)
}
