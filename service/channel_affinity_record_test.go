package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func buildChannelAffinityRecordContextForTest(cacheKey string, ttlSeconds int) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	setChannelAffinityContext(ctx, channelAffinityMeta{
		CacheKey:   cacheKey,
		TTLSeconds: ttlSeconds,
		RuleName:   "record-affinity-test",
	})
	return ctx
}

func withChannelAffinityRecordSetting(t *testing.T) {
	t.Helper()

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)

	originalEnabled := setting.Enabled
	originalSwitchOnSuccess := setting.SwitchOnSuccess
	originalDefaultTTLSeconds := setting.DefaultTTLSeconds

	setting.Enabled = true
	setting.SwitchOnSuccess = false
	setting.DefaultTTLSeconds = 1

	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.SwitchOnSuccess = originalSwitchOnSuccess
		setting.DefaultTTLSeconds = originalDefaultTTLSeconds
	})
}

func newChannelAffinityRecordCacheKey(t *testing.T) string {
	t.Helper()

	cache := GetChannelAffinityCacheForTest()
	cacheKey := cache.FullKey(fmt.Sprintf("record-affinity-test:%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKey})
	})
	return cacheKey
}

func TestRecordChannelAffinityWritesInitialCache(t *testing.T) {
	withChannelAffinityRecordSetting(t)

	cache := GetChannelAffinityCacheForTest()
	cacheKey := newChannelAffinityRecordCacheKey(t)
	ctx := buildChannelAffinityRecordContextForTest(cacheKey, 1)

	RecordChannelAffinity(ctx, 101)

	channelID, found, err := cache.Get(cacheKey)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 101, channelID)
}

func TestInvalidatedChannelAffinityIsNotRecordedAtRequestEnd(t *testing.T) {
	withChannelAffinityRecordSetting(t)

	cache := GetChannelAffinityCacheForTest()
	cacheKey := newChannelAffinityRecordCacheKey(t)
	ctx := buildChannelAffinityRecordContextForTest(cacheKey, 1)
	ctx.Set(ginKeyChannelAffinitySkipRetry, true)
	require.NoError(t, cache.SetWithTTL(cacheKey, 101, time.Minute))

	require.True(t, InvalidateCurrentChannelAffinityCache(ctx))
	require.False(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))
	RecordChannelAffinity(ctx, 202)

	_, found, err := cache.Get(cacheKey)
	require.NoError(t, err)
	require.False(t, found)
}

func TestRecordChannelAffinitySameChannelDoesNotRefreshTTL(t *testing.T) {
	withChannelAffinityRecordSetting(t)

	cache := GetChannelAffinityCacheForTest()
	cacheKey := newChannelAffinityRecordCacheKey(t)
	ctx := buildChannelAffinityRecordContextForTest(cacheKey, 1)

	RecordChannelAffinity(ctx, 101)
	time.Sleep(800 * time.Millisecond)
	RecordChannelAffinity(ctx, 101)
	time.Sleep(450 * time.Millisecond)

	_, found, err := cache.Get(cacheKey)
	require.NoError(t, err)
	require.False(t, found)
}

func TestRecordChannelAffinityDifferentChannelRefreshesTTL(t *testing.T) {
	withChannelAffinityRecordSetting(t)

	cache := GetChannelAffinityCacheForTest()
	cacheKey := newChannelAffinityRecordCacheKey(t)
	ctx := buildChannelAffinityRecordContextForTest(cacheKey, 1)

	RecordChannelAffinity(ctx, 101)
	time.Sleep(800 * time.Millisecond)
	RecordChannelAffinity(ctx, 202)
	time.Sleep(450 * time.Millisecond)

	channelID, found, err := cache.Get(cacheKey)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 202, channelID)

	time.Sleep(700 * time.Millisecond)
	_, found, err = cache.Get(cacheKey)
	require.NoError(t, err)
	require.False(t, found)
}
