package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAnytoolsControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalDataExportEnabled := common.DataExportEnabled
	originalQuotaPerUnit := common.QuotaPerUnit

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))

	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	common.QuotaPerUnit = 500_000

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.DataExportEnabled = originalDataExportEnabled
		common.QuotaPerUnit = originalQuotaPerUnit
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func newAnytoolsControllerTestContext(t *testing.T, path string, body interface{}, userId int, token *model.Token) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		data, err := common.Marshal(body)
		require.NoError(t, err)
		requestBody = bytes.NewReader(data)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, path, requestBody)
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUserId, userId)
	common.SetContextKey(ctx, constant.ContextKeyUserName, "anytools-user")
	common.SetContextKey(ctx, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(ctx, constant.ContextKeyTokenKey, token.Key)
	common.SetContextKey(ctx, constant.ContextKeyTokenUnlimited, token.UnlimitedQuota)
	ctx.Set("token_name", token.Name)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	return ctx, recorder
}

func TestGetAnytoolsBalance(t *testing.T) {
	db := setupAnytoolsControllerTestDB(t)
	tests := []struct {
		name           string
		userQuota      int
		tokenQuota     int
		unlimitedQuota bool
		wantBalance    string
	}{
		{name: "limited by token", userQuota: 1_000_000, tokenQuota: 750_000, wantBalance: "1.5"},
		{name: "negative wallet", userQuota: -1, tokenQuota: 750_000, wantBalance: "-0.000002"},
		{name: "unlimited token", userQuota: 125_000, tokenQuota: -10, unlimitedQuota: true, wantBalance: "0.25"},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user := &model.User{Id: i + 1, Username: fmt.Sprintf("anytools-%d", i), Status: common.UserStatusEnabled, Quota: test.userQuota, AffCode: fmt.Sprintf("anytools-aff-%d", i)}
			token := &model.Token{Id: i + 1, UserId: user.Id, Key: fmt.Sprintf("anytools-key-%d", i), Name: "anytools-token", Status: common.TokenStatusEnabled, RemainQuota: test.tokenQuota, UnlimitedQuota: test.unlimitedQuota}
			require.NoError(t, db.Create(user).Error)
			require.NoError(t, db.Create(token).Error)

			ctx, recorder := newAnytoolsControllerTestContext(t, "/v1/anytools/get-balance", nil, user.Id, token)
			GetAnytoolsBalance(ctx)

			assert.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Balance string `json:"balance"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, test.wantBalance, response.Balance)
		})
	}
}

func TestCheckoutAnytools(t *testing.T) {
	t.Run("successful checkout updates quota statistics and log", func(t *testing.T) {
		db := setupAnytoolsControllerTestDB(t)
		user := &model.User{Id: 1, Username: "anytools-user", Status: common.UserStatusEnabled, Quota: 100, UsedQuota: 10, RequestCount: 2}
		token := &model.Token{Id: 1, UserId: user.Id, Key: "anytools-key", Name: "anytools-token", Status: common.TokenStatusEnabled, RemainQuota: 100, UsedQuota: 20}
		require.NoError(t, db.Create(user).Error)
		require.NoError(t, db.Create(token).Error)

		ctx, recorder := newAnytoolsControllerTestContext(t, "/v1/anytools/checkout", anytoolsCheckoutRequest{
			Cost:  "0.000003",
			Info:  "  billed tool call  ",
			Model: "  external-model  ",
		}, user.Id, token)
		CheckoutAnytools(ctx)

		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.True(t, response.Success)

		var updatedUser model.User
		require.NoError(t, db.First(&updatedUser, user.Id).Error)
		assert.Equal(t, 98, updatedUser.Quota)
		assert.Equal(t, 12, updatedUser.UsedQuota)
		assert.Equal(t, 3, updatedUser.RequestCount)

		var updatedToken model.Token
		require.NoError(t, db.First(&updatedToken, token.Id).Error)
		assert.Equal(t, 98, updatedToken.RemainQuota)
		assert.Equal(t, 22, updatedToken.UsedQuota)

		var logs []model.Log
		require.NoError(t, db.Find(&logs).Error)
		require.Len(t, logs, 1)
		assert.Equal(t, model.LogTypeConsume, logs[0].Type)
		assert.Equal(t, 2, logs[0].Quota)
		assert.Equal(t, "  billed tool call  ", logs[0].Content)
		assert.Equal(t, "external-model", logs[0].ModelName)
		assert.Equal(t, token.Id, logs[0].TokenId)
	})

	t.Run("insufficient API key leaves wallet and usage unchanged", func(t *testing.T) {
		db := setupAnytoolsControllerTestDB(t)
		user := &model.User{Id: 1, Username: "anytools-user", Status: common.UserStatusEnabled, Quota: 100, UsedQuota: 10, RequestCount: 2}
		token := &model.Token{Id: 1, UserId: user.Id, Key: "anytools-key", Name: "anytools-token", Status: common.TokenStatusEnabled, RemainQuota: 1, UsedQuota: 20}
		require.NoError(t, db.Create(user).Error)
		require.NoError(t, db.Create(token).Error)

		ctx, recorder := newAnytoolsControllerTestContext(t, "/v1/anytools/checkout", anytoolsCheckoutRequest{
			Cost: "0.000004", Info: "billed tool call", Model: "external-model",
		}, user.Id, token)
		CheckoutAnytools(ctx)

		assert.Equal(t, http.StatusForbidden, recorder.Code)
		var response struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		assert.Equal(t, "insufficient API key balance", response.Message)

		var updatedUser model.User
		require.NoError(t, db.First(&updatedUser, user.Id).Error)
		assert.Equal(t, 100, updatedUser.Quota)
		assert.Equal(t, 10, updatedUser.UsedQuota)
		assert.Equal(t, 2, updatedUser.RequestCount)
		var updatedToken model.Token
		require.NoError(t, db.First(&updatedToken, token.Id).Error)
		assert.Equal(t, 1, updatedToken.RemainQuota)
		assert.Equal(t, 20, updatedToken.UsedQuota)
		var logCount int64
		require.NoError(t, db.Model(&model.Log{}).Count(&logCount).Error)
		assert.Zero(t, logCount)
	})

	t.Run("insufficient wallet refunds API key and leaves usage unchanged", func(t *testing.T) {
		db := setupAnytoolsControllerTestDB(t)
		user := &model.User{Id: 1, Username: "anytools-user", Status: common.UserStatusEnabled, Quota: 1, UsedQuota: 10, RequestCount: 2}
		token := &model.Token{Id: 1, UserId: user.Id, Key: "anytools-key", Name: "anytools-token", Status: common.TokenStatusEnabled, RemainQuota: 100, UsedQuota: 20}
		require.NoError(t, db.Create(user).Error)
		require.NoError(t, db.Create(token).Error)

		ctx, recorder := newAnytoolsControllerTestContext(t, "/v1/anytools/checkout", anytoolsCheckoutRequest{
			Cost: "0.000004", Info: "billed tool call", Model: "external-model",
		}, user.Id, token)
		CheckoutAnytools(ctx)

		assert.Equal(t, http.StatusForbidden, recorder.Code)
		var response struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		assert.Equal(t, "insufficient wallet balance", response.Message)

		var updatedUser model.User
		require.NoError(t, db.First(&updatedUser, user.Id).Error)
		assert.Equal(t, 1, updatedUser.Quota)
		assert.Equal(t, 10, updatedUser.UsedQuota)
		assert.Equal(t, 2, updatedUser.RequestCount)
		var updatedToken model.Token
		require.NoError(t, db.First(&updatedToken, token.Id).Error)
		assert.Equal(t, 100, updatedToken.RemainQuota)
		assert.Equal(t, 20, updatedToken.UsedQuota)
		var logCount int64
		require.NoError(t, db.Model(&model.Log{}).Count(&logCount).Error)
		assert.Zero(t, logCount)
	})

	t.Run("validates fields and cost", func(t *testing.T) {
		db := setupAnytoolsControllerTestDB(t)
		user := &model.User{Id: 1, Username: "anytools-user", Status: common.UserStatusEnabled, Quota: 100}
		token := &model.Token{Id: 1, UserId: user.Id, Key: "anytools-key", Name: "anytools-token", Status: common.TokenStatusEnabled, RemainQuota: 100}
		require.NoError(t, db.Create(user).Error)
		require.NoError(t, db.Create(token).Error)

		tests := []anytoolsCheckoutRequest{
			{Cost: "", Info: "info", Model: "model"},
			{Cost: "1", Info: " ", Model: "model"},
			{Cost: "1", Info: "info", Model: " "},
			{Cost: "invalid", Info: "info", Model: "model"},
			{Cost: "0", Info: "info", Model: "model"},
			{Cost: "-1", Info: "info", Model: "model"},
			{Cost: "0.0000001", Info: "info", Model: "model"},
			{Cost: "100000000000000000000", Info: "info", Model: "model"},
		}
		for _, test := range tests {
			ctx, recorder := newAnytoolsControllerTestContext(t, "/v1/anytools/checkout", test, user.Id, token)
			CheckoutAnytools(ctx)
			assert.Equal(t, http.StatusBadRequest, recorder.Code, "request: %+v", test)
		}

		var updatedUser model.User
		require.NoError(t, db.First(&updatedUser, user.Id).Error)
		assert.Equal(t, 100, updatedUser.Quota)
		var updatedToken model.Token
		require.NoError(t, db.First(&updatedToken, token.Id).Error)
		assert.Equal(t, 100, updatedToken.RemainQuota)
		var logCount int64
		require.NoError(t, db.Model(&model.Log{}).Count(&logCount).Error)
		assert.Zero(t, logCount)
	})

}
