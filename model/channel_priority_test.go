package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelPriorityTestDB(t *testing.T, memoryCacheEnabled bool) *gorm.DB {
	t.Helper()

	originalDB := DB
	originalLogDB := LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = memoryCacheEnabled

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
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

func insertPriorityTestChannel(t *testing.T, db *gorm.DB, id int, name string, priority int64) {
	t.Helper()

	weight := uint(1)
	require.NoError(t, db.Create(&Channel{
		Id:       id,
		Name:     name,
		Key:      "sk-" + name,
		Status:   common.ChannelStatusEnabled,
		Group:    "default",
		Models:   "gpt-test",
		Priority: common.GetPointer(priority),
		Weight:   &weight,
	}).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "gpt-test",
		ChannelId: id,
		Enabled:   true,
		Priority:  common.GetPointer(priority),
		Weight:    weight,
	}).Error)
}

func insertAdvancedCustomPriorityTestChannel(t *testing.T, db *gorm.DB, id int, name string, model string) {
	t.Helper()

	weight := uint(1)
	channel := &Channel{
		Id:       id,
		Type:     constant.ChannelTypeAdvancedCustom,
		Name:     name,
		Key:      "sk-" + name,
		Status:   common.ChannelStatusEnabled,
		Group:    "default",
		Models:   "gpt-test",
		Priority: common.GetPointer(int64(0)),
		Weight:   &weight,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/responses",
			UpstreamPath: "/v1/responses",
			Models:       []string{model},
		}}},
	})
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "gpt-test",
		ChannelId: id,
		Enabled:   true,
		Priority:  common.GetPointer(int64(0)),
		Weight:    weight,
	}).Error)
}

func TestDeprioritizeFailedChannelUpdatesChannelAndAbilityWhenPriorityNonPositive(t *testing.T) {
	db := setupChannelPriorityTestDB(t, false)
	insertPriorityTestChannel(t, db, 1, "failed", 0)
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", 1).Updates(map[string]any{
		"weight": 99,
		"tag":    "preserve",
	}).Error)

	before := common.GetTimestamp()
	updated, err := DeprioritizeFailedChannel(1)
	after := common.GetTimestamp()
	require.NoError(t, err)
	require.True(t, updated)

	channel, err := GetChannelById(1, true)
	require.NoError(t, err)
	require.NotNil(t, channel.Priority)

	expectedMax := failedChannelPriorityBase - before
	expectedMin := failedChannelPriorityBase - after
	require.LessOrEqual(t, *channel.Priority, expectedMax)
	require.GreaterOrEqual(t, *channel.Priority, expectedMin)

	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ?", 1).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.NotNil(t, abilities[0].Priority)
	assert.Equal(t, *channel.Priority, *abilities[0].Priority)
	assert.Equal(t, uint(99), abilities[0].Weight)
	require.NotNil(t, abilities[0].Tag)
	assert.Equal(t, "preserve", *abilities[0].Tag)
}

func TestDeprioritizeFailedChannelSkipsPositivePriority(t *testing.T) {
	db := setupChannelPriorityTestDB(t, false)
	insertPriorityTestChannel(t, db, 2, "positive", 9)

	updated, err := DeprioritizeFailedChannel(2)
	require.NoError(t, err)
	require.False(t, updated)

	channel, err := GetChannelById(2, true)
	require.NoError(t, err)
	require.NotNil(t, channel.Priority)
	assert.Equal(t, int64(9), *channel.Priority)

	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ?", 2).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.NotNil(t, abilities[0].Priority)
	assert.Equal(t, int64(9), *abilities[0].Priority)
}

func TestDeprioritizeFailedChannelRefreshesMemoryCacheSelection(t *testing.T) {
	db := setupChannelPriorityTestDB(t, true)
	insertPriorityTestChannel(t, db, 11, "primary", 0)
	insertPriorityTestChannel(t, db, 12, "backup", -100)
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test", 0, "", nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 11, channel.Id)

	updated, err := DeprioritizeFailedChannel(11)
	require.NoError(t, err)
	require.True(t, updated)

	channel, err = GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test", 0, "", nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)

	priorityCount, err := GetSatisfiedChannelPriorityCount("default", "gpt-test", "gpt-test", "", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, priorityCount)
}

func TestGetRandomSatisfiedChannelUsesNextHighestPriorityAfterExclusionWithCache(t *testing.T) {
	db := setupChannelPriorityTestDB(t, true)
	insertPriorityTestChannel(t, db, 21, "high", 20)
	insertPriorityTestChannel(t, db, 22, "mid", 10)
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test", 1, "", map[int]struct{}{21: {}})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 22, channel.Id)
}

func TestGetRandomSatisfiedChannelUsesRemainingSamePriorityAfterExclusionWithCache(t *testing.T) {
	db := setupChannelPriorityTestDB(t, true)
	insertPriorityTestChannel(t, db, 31, "a", 0)
	insertPriorityTestChannel(t, db, 32, "b", 0)
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test", 1, "", map[int]struct{}{31: {}})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 32, channel.Id)
}

func TestGetRandomSatisfiedChannelUsesNextHighestPriorityAfterExclusionWithoutCache(t *testing.T) {
	db := setupChannelPriorityTestDB(t, false)
	insertPriorityTestChannel(t, db, 41, "high", 20)
	insertPriorityTestChannel(t, db, 42, "mid", 10)

	channel, err := GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test", 1, "", map[int]struct{}{41: {}})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 42, channel.Id)
}

func TestGetSatisfiedChannelPriorityCountFiltersExcludedChannels(t *testing.T) {
	db := setupChannelPriorityTestDB(t, true)
	insertPriorityTestChannel(t, db, 51, "high", 20)
	insertPriorityTestChannel(t, db, 52, "mid", 10)
	InitChannelCache()

	priorityCount, err := GetSatisfiedChannelPriorityCount("default", "gpt-test", "gpt-test", "", map[int]struct{}{51: {}})
	require.NoError(t, err)
	assert.Equal(t, 1, priorityCount)

	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = true
	})
	priorityCount, err = GetSatisfiedChannelPriorityCount("default", "gpt-test", "gpt-test", "", map[int]struct{}{51: {}})
	require.NoError(t, err)
	assert.Equal(t, 1, priorityCount)
}

func TestAdvancedCustomSelectionFiltersRequestModelWithAndWithoutCache(t *testing.T) {
	db := setupChannelPriorityTestDB(t, true)
	insertAdvancedCustomPriorityTestChannel(t, db, 61, "wrong-model", "other-model")
	insertAdvancedCustomPriorityTestChannel(t, db, 62, "matching-model", "gpt-test")
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test", 0, "/v1/responses", nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 62, channel.Id)

	common.MemoryCacheEnabled = false
	channel, err = GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test", 0, "/v1/responses", nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 62, channel.Id)
}

func TestAdvancedCustomSelectionUsesFullModelForRouteAndBaseModelForAbility(t *testing.T) {
	db := setupChannelPriorityTestDB(t, true)
	insertAdvancedCustomPriorityTestChannel(t, db, 63, "wrong-suffix", "gpt-test")
	insertAdvancedCustomPriorityTestChannel(t, db, 64, "matching-suffix", "gpt-test-xhigh")
	InitChannelCache()

	channel, err := GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test-xhigh", 0, "/v1/responses", nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 64, channel.Id)

	common.MemoryCacheEnabled = false
	channel, err = GetRandomSatisfiedChannel("default", "gpt-test", "gpt-test-xhigh", 0, "/v1/responses", nil)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 64, channel.Id)
}

func TestChannelSupportsRequestPathAndModelForAffinity(t *testing.T) {
	db := setupChannelPriorityTestDB(t, true)
	insertAdvancedCustomPriorityTestChannel(t, db, 71, "affinity", "gpt-test")
	InitChannelCache()

	channel, err := CacheGetChannel(71)
	require.NoError(t, err)
	assert.True(t, ChannelSupportsRequestPathAndModel(channel, "/v1/responses", "gpt-test"))
	assert.False(t, ChannelSupportsRequestPathAndModel(channel, "/v1/responses", "other-model"))
}
