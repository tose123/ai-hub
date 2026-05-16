package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAutoGroupRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func withRuntimeAutoGroups(t *testing.T, groups []string) {
	t.Helper()

	original := append([]string(nil), setting.GetAutoGroups()...)
	data, err := common.Marshal(groups)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(string(data)))
	t.Cleanup(func() {
		restore, err := common.Marshal(original)
		require.NoError(t, err)
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(string(restore)))
	})
}

func seedRuntimeAutoGroupData(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id: 1, Username: "runtime-user", Password: "password", Group: "default", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 101, Name: "default-channel", Key: "sk-default", Status: common.ChannelStatusEnabled, Group: "default", Models: "runtime-model"},
		{Id: 202, Name: "vip-channel", Key: "sk-vip", Status: common.ChannelStatusEnabled, Group: "vip", Models: "runtime-model"},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "runtime-model", ChannelId: 101, Enabled: true},
		{Group: "vip", Model: "runtime-model", ChannelId: 202, Enabled: true},
	}).Error)
	model.InitChannelCache()
}

func buildRuntimeAutoGroupContext(userGroup string, override []string) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, userGroup)
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, false)
	if override != nil {
		common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroupsOverride, override)
	}
	return ctx
}

func TestCacheGetRandomSatisfiedChannelFallsBackToSystemAutoGroups(t *testing.T) {
	db := setupAutoGroupRuntimeTestDB(t)
	withRuntimeAutoGroups(t, []string{"default", "vip"})
	seedRuntimeAutoGroupData(t, db)

	ctx := buildRuntimeAutoGroupContext("default", nil)
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx: ctx, TokenGroup: "auto", ModelName: "runtime-model", Retry: common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 101, channel.Id)
	require.Equal(t, "default", selectedGroup)
	require.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestCacheGetRandomSatisfiedChannelUsesTokenOverrideOrder(t *testing.T) {
	db := setupAutoGroupRuntimeTestDB(t)
	withRuntimeAutoGroups(t, []string{"default", "vip"})
	seedRuntimeAutoGroupData(t, db)

	ctx := buildRuntimeAutoGroupContext("default", []string{"vip"})
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx: ctx, TokenGroup: "auto", ModelName: "runtime-model", Retry: common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 202, channel.Id)
	require.Equal(t, "vip", selectedGroup)
	require.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}
