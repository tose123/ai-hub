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
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	RequestPath  string
	Retry        *int
	resetNextTry bool
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
// 普通分组直接按 retry 映射到优先级层级。auto 分组会按 token 可用的
// autoGroups 顺序选择；每个子分组最多尝试
// min(优先级层级数, RetryTimes+1) 次。开启跨组重试时，组内优先级耗尽后
// 切到下一组，并把组内 retry 重置为 0；未开启时则在当前组耗尽后停止继续
// 尝试后续分组。亲和性首发失败后的第一次兜底也从 autoGroups 的第一个分组、
// 最高优先级重新开始。
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
			// Calculate priorityRetry for current group
			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			priorityCount, priorityErr := model.GetSatisfiedChannelPriorityCount(autoGroup, param.ModelName)
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

			channel, _ = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, priorityRetry, param.RequestPath)
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= groupRetryLimit-1 {
				// Current group has exhausted all retries, prepare to switch to next group
				// This request still uses current group, but next retry will use next group
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s priority retries exhausted (priorityRetry=%d >= limit=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, groupRetryLimit)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), param.RequestPath)
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
