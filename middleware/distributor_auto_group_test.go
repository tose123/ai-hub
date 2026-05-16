package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDistributorAutoGroupTestDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func withDistributorAutoGroups(t *testing.T, groups []string) {
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

func seedDistributorAutoGroupData(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&model.Channel{Id: 300, Name: "affinity-channel", Key: "sk-affinity", Status: common.ChannelStatusEnabled, Group: "default,vip", Models: "runtime-model"}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "runtime-model", ChannelId: 300, Enabled: true},
		{Group: "vip", Model: "runtime-model", ChannelId: 300, Enabled: true},
	}).Error)
	model.InitChannelCache()
}

func buildDistributorContext(override []string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := strings.NewReader(`{"model":"runtime-model"}`)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, false)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, false)
	if override != nil {
		common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroupsOverride, override)
	}
	return ctx, rec
}

func TestDistributorAutoGroupFallbackUsesHelperState(t *testing.T) {
	db := setupDistributorAutoGroupTestDB(t)
	withDistributorAutoGroups(t, []string{"default", "vip"})
	seedDistributorAutoGroupData(t, db)
	ctx, rec := buildDistributorContext(nil)

	Distribute()(ctx)

	require.Less(t, rec.Code, http.StatusBadRequest)
	require.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	require.Equal(t, 300, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
}

func TestDistributorAutoGroupOverrideChangesSelectedGroup(t *testing.T) {
	db := setupDistributorAutoGroupTestDB(t)
	withDistributorAutoGroups(t, []string{"default", "vip"})
	seedDistributorAutoGroupData(t, db)
	ctx, rec := buildDistributorContext([]string{"vip"})

	Distribute()(ctx)

	require.Less(t, rec.Code, http.StatusBadRequest)
	require.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	require.Equal(t, 300, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
}

func TestDistributorAffinityBranchUsesTokenAwareAutoGroups(t *testing.T) {
	db := setupDistributorAutoGroupTestDB(t)
	withDistributorAutoGroups(t, []string{"default", "vip"})
	seedDistributorAutoGroupData(t, db)
	ctx, rec := buildDistributorContext([]string{"vip"})	
	ctx.Request.Header.Set("User-Agent", "Codex CLI")
	body := strings.NewReader(`{"model":"runtime-model","prompt_cache_key":"pc-hit"}`)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("User-Agent", "Codex CLI")
	cache := service.GetChannelAffinityCacheForTest()
	require.NoError(t, cache.SetWithTTL("codex cli trace:runtime-model:auto:pc-hit", 300, 0))

	Distribute()(ctx)

	require.Less(t, rec.Code, http.StatusBadRequest)
	require.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	require.Equal(t, 300, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
}
