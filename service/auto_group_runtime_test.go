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
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
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
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
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

func withRuntimeUserUsableGroups(t *testing.T, groups []string) {
	t.Helper()

	original := setting.UserUsableGroups2JSONString()
	groupMap := make(map[string]string, len(groups))
	for _, group := range groups {
		groupMap[group] = group
	}
	data, err := common.Marshal(groupMap)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(data)))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(original))
	})
}

func withRuntimeRetryTimes(t *testing.T, retryTimes int) {
	t.Helper()

	original := common.RetryTimes
	common.RetryTimes = retryTimes
	t.Cleanup(func() {
		common.RetryTimes = original
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

func seedPriorityAutoGroupData(t *testing.T, db *gorm.DB, modelName string, groupPriorities map[string][]int64) {
	t.Helper()

	channelID := 1000
	for group, priorities := range groupPriorities {
		for _, priority := range priorities {
			channelID++
			priorityValue := priority
			weightValue := uint(5)
			channelName := fmt.Sprintf("%s_C%d", group, priority)
			require.NoError(t, db.Create(&model.Channel{
				Id:       channelID,
				Name:     channelName,
				Key:      fmt.Sprintf("sk-%s", channelName),
				Status:   common.ChannelStatusEnabled,
				Group:    group,
				Models:   modelName,
				Priority: &priorityValue,
				Weight:   &weightValue,
			}).Error)
			require.NoError(t, db.Create(&model.Ability{
				Group:     group,
				Model:     modelName,
				ChannelId: channelID,
				Enabled:   true,
				Priority:  &priorityValue,
				Weight:    weightValue,
			}).Error)
		}
	}
	model.InitChannelCache()
}

func collectAutoGroupSelections(t *testing.T, param *RetryParam, maxAttempts int) []string {
	t.Helper()

	names := make([]string, 0, maxAttempts)
	for i := 0; i < maxAttempts; i++ {
		channel, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		if channel == nil {
			break
		}
		names = append(names, channel.Name)
		param.IncreaseRetry()
	}
	return names
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

func TestCacheGetRandomSatisfiedChannelCrossGroupExhaustsPrioritiesOnce(t *testing.T) {
	db := setupAutoGroupRuntimeTestDB(t)
	withRuntimeAutoGroups(t, []string{"G1", "G2", "G3"})
	withRuntimeUserUsableGroups(t, []string{"G1", "G2", "G3"})
	withRuntimeRetryTimes(t, 5)
	seedPriorityAutoGroupData(t, db, "runtime-model", map[string][]int64{
		"G1": {9, 8, 7},
		"G2": {9, 8, 7},
		"G3": {9, 8, 7},
	})

	ctx := buildRuntimeAutoGroupContext("default", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "auto", ModelName: "runtime-model", Retry: common.GetPointer(0),
	}

	names := collectAutoGroupSelections(t, param, 12)

	require.Equal(t, []string{
		"G1_C9", "G1_C8", "G1_C7",
		"G2_C9", "G2_C8", "G2_C7",
		"G3_C9", "G3_C8", "G3_C7",
	}, names)
}

func TestCacheGetRandomSatisfiedChannelAffinityFallbackStartsAtFirstPriority(t *testing.T) {
	db := setupAutoGroupRuntimeTestDB(t)
	withRuntimeAutoGroups(t, []string{"G1", "G2", "G3"})
	withRuntimeUserUsableGroups(t, []string{"G1", "G2", "G3"})
	withRuntimeRetryTimes(t, 5)
	seedPriorityAutoGroupData(t, db, "runtime-model", map[string][]int64{
		"G1": {9, 8, 7},
		"G2": {9, 8, 7},
		"G3": {9, 8, 7},
	})

	ctx := buildRuntimeAutoGroupContext("default", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	setChannelAffinityContext(ctx, channelAffinityMeta{
		RuleName:   "test-affinity",
		UsingGroup: "auto",
		ModelName:  "runtime-model",
	})
	MarkChannelAffinityUsed(ctx, "G2", 1)
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "auto", ModelName: "runtime-model", Retry: common.GetPointer(1),
	}

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, "G1", selectedGroup)
	require.Equal(t, "G1_C9", channel.Name)
	require.Equal(t, 0, param.GetRetry())

	param.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, "G1", selectedGroup)
	require.Equal(t, "G1_C8", channel.Name)
}

func TestCacheGetRandomSatisfiedChannelCrossGroupSkipsEmptyGroup(t *testing.T) {
	db := setupAutoGroupRuntimeTestDB(t)
	withRuntimeAutoGroups(t, []string{"G1", "G2"})
	withRuntimeUserUsableGroups(t, []string{"G1", "G2"})
	withRuntimeRetryTimes(t, 5)
	seedPriorityAutoGroupData(t, db, "runtime-model", map[string][]int64{
		"G1": {},
		"G2": {9},
	})

	ctx := buildRuntimeAutoGroupContext("default", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx: ctx, TokenGroup: "auto", ModelName: "runtime-model", Retry: common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, "G2", selectedGroup)
	require.Equal(t, "G2_C9", channel.Name)
}

func TestCacheGetRandomSatisfiedChannelWithoutCrossGroupRetryCapsByPriorityCount(t *testing.T) {
	db := setupAutoGroupRuntimeTestDB(t)
	withRuntimeAutoGroups(t, []string{"G1"})
	withRuntimeUserUsableGroups(t, []string{"G1"})
	withRuntimeRetryTimes(t, 5)
	seedPriorityAutoGroupData(t, db, "runtime-model", map[string][]int64{
		"G1": {9, 8},
	})

	ctx := buildRuntimeAutoGroupContext("default", nil)
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "auto", ModelName: "runtime-model", Retry: common.GetPointer(0),
	}

	names := collectAutoGroupSelections(t, param, 5)

	require.Equal(t, []string{"G1_C9", "G1_C8"}, names)
}

func TestCacheGetRandomSatisfiedChannelWithoutCrossGroupRetryStopsAfterCurrentGroup(t *testing.T) {
	db := setupAutoGroupRuntimeTestDB(t)
	withRuntimeAutoGroups(t, []string{"G1", "G2"})
	withRuntimeUserUsableGroups(t, []string{"G1", "G2"})
	withRuntimeRetryTimes(t, 5)
	seedPriorityAutoGroupData(t, db, "runtime-model", map[string][]int64{
		"G1": {9, 8},
		"G2": {9, 8},
	})

	ctx := buildRuntimeAutoGroupContext("default", nil)
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "auto", ModelName: "runtime-model", Retry: common.GetPointer(2),
	}

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Nil(t, channel)
	require.Equal(t, "G1", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelCrossGroupCapsPrioritiesByRetryTimes(t *testing.T) {
	db := setupAutoGroupRuntimeTestDB(t)
	withRuntimeAutoGroups(t, []string{"G1"})
	withRuntimeUserUsableGroups(t, []string{"G1"})
	withRuntimeRetryTimes(t, 1)
	seedPriorityAutoGroupData(t, db, "runtime-model", map[string][]int64{
		"G1": {9, 8, 7},
	})

	ctx := buildRuntimeAutoGroupContext("default", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	param := &RetryParam{
		Ctx: ctx, TokenGroup: "auto", ModelName: "runtime-model", Retry: common.GetPointer(0),
	}

	names := collectAutoGroupSelections(t, param, 5)

	require.Equal(t, []string{"G1_C9", "G1_C8"}, names)
}
