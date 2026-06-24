package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx                   *gin.Context
	TokenGroup            string
	ModelName             string
	RequestPath           string
	Retry                 *int
	ExcludedChannelIDs    map[int]struct{}
	currentAutoGroup      string
	currentAutoGroupLimit int
	resetNextTry          bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

// CacheGetRandomSatisfiedChannel selects a channel for the given group/model.
//
// 普通分组会过滤掉本请求已尝试的渠道，并从剩余候选中仅选择当前最高优先级层。
// auto 分组会在当前分组仍有剩余候选时持续留在当前分组；只有当前分组耗尽后，
// 才会根据 crossGroupRetry 决定是否切到下一组。亲和性首发失败后的第一次兜底
// 也从 autoGroups 的第一个分组、最高优先级重新开始。
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetTokenAwareAutoGroups(param.Ctx, userGroup)

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
		hasAutoGroupIndex := false

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
				hasAutoGroupIndex = true
			}
		}
		if crossGroupRetry && !hasAutoGroupIndex && param.GetRetry() > 0 && channelAffinitySelected(param.Ctx) {
			param.SetRetry(0)
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			selectGroup = autoGroup
			if param.ExcludedChannelIDs == nil {
				remainingCount, countErr := model.GetSatisfiedChannelCount(autoGroup, param.ModelName, param.RequestPath, nil)
				if countErr != nil {
					return nil, selectGroup, countErr
				}
				priorityRetry := param.GetRetry()
				if i > startGroupIndex {
					priorityRetry = 0
				}
				priorityCount, priorityErr := model.GetSatisfiedChannelPriorityCount(autoGroup, param.ModelName, param.RequestPath, nil)
				if priorityErr != nil {
					return nil, selectGroup, priorityErr
				}
				groupRetryLimit := getAutoGroupRetryLimit(priorityCount)
				if groupRetryLimit == 0 {
					logger.LogDebug(param.Ctx, "No priority in group %s for model %s, trying next group", autoGroup, param.ModelName)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
					param.SetRetry(0)
					continue
				}
				if priorityRetry >= groupRetryLimit {
					logger.LogDebug(param.Ctx, "Auto group %s priority retries exhausted (priorityRetry=%d >= limit=%d)", autoGroup, priorityRetry, groupRetryLimit)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
					if crossGroupRetry {
						logger.LogDebug(param.Ctx, "Trying next group after exhausting auto group %s", autoGroup)
						common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
						param.SetRetry(0)
						continue
					}
					return nil, selectGroup, nil
				}
				logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

				channel, err = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, priorityRetry, param.RequestPath, nil)
				if err != nil {
					return nil, selectGroup, err
				}
				if channel == nil {
					logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
					param.SetRetry(0)
					continue
				}
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
				param.currentAutoGroup = autoGroup
				param.currentAutoGroupLimit = getAutoGroupRetryLimit(remainingCount)
				logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

				if crossGroupRetry && priorityRetry >= groupRetryLimit-1 {
					logger.LogDebug(param.Ctx, "Current group %s priority retries exhausted (priorityRetry=%d >= limit=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, groupRetryLimit)
					param.currentAutoGroup = ""
					param.currentAutoGroupLimit = 0
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
					param.SetRetry(0)
					param.ResetRetryNextTry()
				} else {
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
				}
				break
			}

			groupRetry := param.GetRetry()
			if i > startGroupIndex {
				groupRetry = 0
			}

			remainingCount, countErr := model.GetSatisfiedChannelCount(autoGroup, param.ModelName, param.RequestPath, param.ExcludedChannelIDs)
			if countErr != nil {
				return nil, selectGroup, countErr
			}
			priorityCount, priorityErr := model.GetSatisfiedChannelPriorityCount(autoGroup, param.ModelName, param.RequestPath, param.ExcludedChannelIDs)
			if priorityErr != nil {
				return nil, selectGroup, priorityErr
			}

			if remainingCount == 0 {
				logger.LogDebug(param.Ctx, "No remaining channel in group %s for model %s, trying next group", autoGroup, param.ModelName)
				param.currentAutoGroup = ""
				param.currentAutoGroupLimit = 0
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				if crossGroupRetry {
					param.SetRetry(0)
				} else if i == startGroupIndex {
					return nil, selectGroup, nil
				}
				continue
			}

			if param.currentAutoGroup != autoGroup || param.currentAutoGroupLimit <= 0 {
				param.currentAutoGroup = autoGroup
				param.currentAutoGroupLimit = getAutoGroupRetryLimit(remainingCount)
			}
			groupRetryLimit := param.currentAutoGroupLimit
			if groupRetry >= groupRetryLimit {
				logger.LogDebug(param.Ctx, "Auto group %s retries exhausted (groupRetry=%d >= limit=%d, remainingPriorityCount=%d)", autoGroup, groupRetry, groupRetryLimit, priorityCount)
				param.currentAutoGroup = ""
				param.currentAutoGroupLimit = 0
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				if crossGroupRetry {
					logger.LogDebug(param.Ctx, "Trying next group after exhausting auto group %s", autoGroup)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
					param.SetRetry(0)
					continue
				}
				return nil, selectGroup, nil
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, groupRetry: %d, remainingPriorityCount: %d", autoGroup, groupRetry, priorityCount)

			channel, err = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, groupRetry, param.RequestPath, param.ExcludedChannelIDs)
			if err != nil {
				return nil, selectGroup, err
			}
			if channel == nil {
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s after filtering, trying next group", autoGroup, param.ModelName)
				param.currentAutoGroup = ""
				param.currentAutoGroupLimit = 0
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				if crossGroupRetry {
					param.SetRetry(0)
				} else if i == startGroupIndex {
					return nil, selectGroup, nil
				}
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && groupRetry >= groupRetryLimit-1 {
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (groupRetry=%d >= limit=%d), preparing switch to next group for next retry", autoGroup, groupRetry, groupRetryLimit)
				param.currentAutoGroup = ""
				param.currentAutoGroupLimit = 0
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), param.RequestPath, param.ExcludedChannelIDs)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}

func getAutoGroupRetryLimit(priorityCount int) int {
	if priorityCount <= 0 {
		return 0
	}
	retryLimit := common.RetryTimes + 1
	if retryLimit <= 0 || priorityCount < retryLimit {
		return priorityCount
	}
	return retryLimit
}
