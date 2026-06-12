package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestShouldRetryClientCanceledDoesNotRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	reqCtx, cancel := context.WithCancel(req.Context())
	cancel()
	ctx.Request = req.WithContext(reqCtx)

	require.False(t, shouldRetry(ctx, types.NewClientCanceledError(context.Canceled), 3))
	require.False(t, shouldRetry(ctx, types.NewError(errors.New("channel error"), types.ErrorCodeChannelInvalidKey), 3))
}

func TestShouldRetryRegular500StillRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	apiErr := types.NewErrorWithStatusCode(errors.New("upstream error"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

	require.True(t, shouldRetry(ctx, apiErr, 1))
}

func TestRecordRelayErrorLogClientCanceled(t *testing.T) {
	db := setupRelayCancelControllerTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		constant.ErrorLogEnabled = originalErrorLogEnabled
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("User-Agent", "relay-cancel-test")
	ctx.Set("id", 2)
	ctx.Set("username", "alice")
	ctx.Set("token_name", "default")
	ctx.Set("token_id", 18)
	ctx.Set("original_model", "gpt-5.5")
	ctx.Set("group", "auto")
	ctx.Set("channel_id", 5)
	ctx.Set("channel_name", "primary")
	ctx.Set("channel_type", 1)
	ctx.Set("use_channel", []string{"5"})
	common.SetContextKey(ctx, constant.ContextKeyEstimatedTokens, 12345)

	recordRelayErrorLog(ctx, types.NewClientCanceledError(context.Canceled))

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeError, logs[0].Type)
	require.Equal(t, 2, logs[0].UserId)
	require.Equal(t, 5, logs[0].ChannelId)
	require.Equal(t, "gpt-5.5", logs[0].ModelName)
	require.Equal(t, "default", logs[0].TokenName)
	require.Equal(t, 12345, logs[0].PromptTokens)
	require.Equal(t, 0, logs[0].CompletionTokens)
	require.Equal(t, "status_code=499, client canceled: context canceled", logs[0].Content)

	var other map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(logs[0].Other), &other))
	require.Equal(t, "client_canceled", other["error_code"])
	require.Equal(t, float64(types.StatusClientClosedRequest), other["status_code"])
	require.Equal(t, "/v1/responses", other["request_path"])
	require.Equal(t, true, other["client_canceled"])
	require.Equal(t, "relay-cancel-test", other["user_agent"])
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"5"}, adminInfo["use_channel"])

	userLogs, total, err := model.GetUserLogs(2, model.LogTypeError, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, userLogs, 1)
	var userOther map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(userLogs[0].Other), &userOther))
	require.Equal(t, true, userOther["client_canceled"])
	require.NotContains(t, userOther, "user_agent")
}

func TestRecordRelayErrorLogUsesPromptTokensFallback(t *testing.T) {
	db := setupRelayCancelControllerTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		constant.ErrorLogEnabled = originalErrorLogEnabled
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 2)
	ctx.Set("username", "alice")
	ctx.Set("token_name", "default")
	ctx.Set("token_id", 18)
	ctx.Set("original_model", "gpt-5.5")
	ctx.Set("group", "auto")
	ctx.Set("channel_id", 5)
	ctx.Set("channel_name", "primary")
	ctx.Set("channel_type", 1)
	ctx.Set("use_channel", []string{"5"})
	common.SetContextKey(ctx, constant.ContextKeyPromptTokens, 6789)

	recordRelayErrorLog(ctx, types.NewErrorWithStatusCode(errors.New("upstream context length exceeded"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest))

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, 6789, logs[0].PromptTokens)
	require.Equal(t, 0, logs[0].CompletionTokens)
}

func TestRecordRelayErrorLogUsesRequestModelWhenMapped(t *testing.T) {
	db := setupRelayCancelControllerTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		constant.ErrorLogEnabled = originalErrorLogEnabled
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 2)
	ctx.Set("username", "alice")
	ctx.Set("token_name", "default")
	ctx.Set("token_id", 18)
	ctx.Set("original_model", "gpt-5.5")
	ctx.Set("group", "auto")
	ctx.Set("channel_id", 5)
	ctx.Set("channel_name", "primary")
	ctx.Set("channel_type", 1)
	ctx.Set("use_channel", []string{"5"})
	common.SetContextKey(ctx, constant.ContextKeyRequestModel, "alias-a")
	common.SetContextKey(ctx, constant.ContextKeyUpstreamModel, "gpt-5.5")

	recordRelayErrorLog(ctx, types.NewErrorWithStatusCode(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway))

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, "alias-a", logs[0].ModelName)
	var other map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(logs[0].Other), &other))
	require.Equal(t, true, other["is_model_mapped"])
	require.Equal(t, "gpt-5.5", other["upstream_model_name"])
	require.Equal(t, "alias-a", other["request_model_name"])
}

func TestRecordRelayErrorLogUsesFinalUpstreamModelWhenDoubleMapped(t *testing.T) {
	db := setupRelayCancelControllerTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		constant.ErrorLogEnabled = originalErrorLogEnabled
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 2)
	ctx.Set("username", "alice")
	ctx.Set("token_name", "default")
	ctx.Set("token_id", 18)
	ctx.Set("original_model", "model-b")
	ctx.Set("group", "auto")
	ctx.Set("channel_id", 5)
	ctx.Set("channel_name", "primary")
	ctx.Set("channel_type", 1)
	ctx.Set("use_channel", []string{"5"})
	common.SetContextKey(ctx, constant.ContextKeyRequestModel, "alias-a")
	common.SetContextKey(ctx, constant.ContextKeyUpstreamModel, "model-c")

	recordRelayErrorLog(ctx, types.NewErrorWithStatusCode(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway))

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, "alias-a", logs[0].ModelName)
	var other map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(logs[0].Other), &other))
	require.Equal(t, true, other["is_model_mapped"])
	require.Equal(t, "model-c", other["upstream_model_name"])
}

func TestRecordConsumeLogAddsRequestMetadata(t *testing.T) {
	db := setupRelayCancelControllerTestDB(t)
	originalLogConsumeEnabled := common.LogConsumeEnabled
	t.Cleanup(func() {
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})
	common.LogConsumeEnabled = true

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("User-Agent", "consume-log-test")
	ctx.Set("username", "alice")

	model.RecordConsumeLog(ctx, 2, model.RecordConsumeLogParams{
		ChannelId:        5,
		PromptTokens:     10,
		CompletionTokens: 20,
		ModelName:        "gpt-5.5",
		TokenName:        "default",
		Quota:            30,
		Content:          "consume",
		TokenId:          18,
		UseTimeSeconds:   1,
		IsStream:         true,
		Group:            "auto",
		Other: map[string]interface{}{
			"existing": "ok",
		},
	})

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	var other map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(logs[0].Other), &other))
	require.Equal(t, false, other["client_canceled"])
	require.Equal(t, "consume-log-test", other["user_agent"])
	require.Equal(t, "ok", other["existing"])
}

func setupRelayCancelControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLOGDB
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.RedisEnabled = originalRedisEnabled

		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}
