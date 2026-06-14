package service

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	inviteCashbackTaskName      = "invite_cashback_daily"
	inviteCashbackDateLayout    = "2006-01-02"
	inviteCashbackTickInterval  = 1 * time.Minute
	inviteCashbackBatchSize     = 300
	inviteCashbackTriggerHour   = 4
	inviteCashbackBillingSource = "wallet"
)

var (
	inviteCashbackOnce    sync.Once
	inviteCashbackRunning atomic.Bool
)

func StartInviteCashbackTask() {
	inviteCashbackOnce.Do(func() {
		gopool.Go(func() {
			common.SysLog(fmt.Sprintf("invite cashback task started: tick=%s", inviteCashbackTickInterval))
			for {
				time.Sleep(inviteCashbackTickInterval)
				runInviteCashbackOnce()
			}
		})
	})
}

func runInviteCashbackOnce() {
	if !inviteCashbackRunning.CompareAndSwap(false, true) {
		return
	}
	defer inviteCashbackRunning.Store(false)

	if common.InviteCashbackPercent <= 0 {
		return
	}
	if !operation_setting.IsPaymentComplianceConfirmed() {
		return
	}

	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		common.SysLog("failed to load Asia/Shanghai location: " + err.Error())
		return
	}

	now := time.Now().In(location)
	triggerAt := time.Date(now.Year(), now.Month(), now.Day(), inviteCashbackTriggerHour, 0, 0, 0, location)
	if now.Before(triggerAt) {
		return
	}

	targetDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -1)

	initialLastCompletedDate := targetDate.AddDate(0, 0, -1).Format(inviteCashbackDateLayout)
	state, initialized, err := model.GetOrCreateInviteCashbackTaskState(inviteCashbackTaskName, initialLastCompletedDate)
	if err != nil {
		common.SysLog("failed to load invite cashback task state: " + err.Error())
		return
	}
	if initialized {
		common.SysLog(fmt.Sprintf("invite cashback task initialized with last_completed_date=%s", initialLastCompletedDate))
	}

	lastCompletedDate, err := time.ParseInLocation(inviteCashbackDateLayout, state.LastCompletedDate, location)
	if err != nil {
		common.SysLog("failed to parse invite cashback last completed date: " + err.Error())
		return
	}

	for currentDate := lastCompletedDate.AddDate(0, 0, 1); !currentDate.After(targetDate); currentDate = currentDate.AddDate(0, 0, 1) {
		if err := processInviteCashbackDay(currentDate, location); err != nil {
			common.SysLog(fmt.Sprintf("invite cashback task failed for %s: %s", currentDate.Format(inviteCashbackDateLayout), err.Error()))
			return
		}
		if err := model.UpdateInviteCashbackTaskState(inviteCashbackTaskName, currentDate.Format(inviteCashbackDateLayout)); err != nil {
			common.SysLog("failed to update invite cashback task state: " + err.Error())
			return
		}
	}
}

func processInviteCashbackDay(day time.Time, location *time.Location) error {
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	dayEnd := dayStart.AddDate(0, 0, 1)
	statDate := dayStart.Format(inviteCashbackDateLayout)

	cursor := 0
	rewardedUsers := 0
	totalCashback := 0

	for {
		users, err := model.ListInviteeRelationsAfterID(cursor, inviteCashbackBatchSize)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			break
		}

		for _, user := range users {
			cursor = user.Id

			netConsumed, err := model.SumInviteeWalletNetQuota(user.Id, dayStart.Unix(), dayEnd.Unix())
			if err != nil {
				return err
			}
			if netConsumed <= 0 {
				continue
			}

			cashbackQuota := netConsumed * common.InviteCashbackPercent / 100
			if cashbackQuota <= 0 {
				continue
			}

			applied, err := model.ApplyInviteCashback(
				statDate,
				user.Id,
				user.InviterId,
				netConsumed,
				common.InviteCashbackPercent,
				cashbackQuota,
			)
			if err != nil {
				return err
			}
			if applied {
				rewardedUsers++
				totalCashback += cashbackQuota
			}
		}

		if len(users) < inviteCashbackBatchSize {
			break
		}
	}

	if rewardedUsers > 0 {
		common.SysLog(
			fmt.Sprintf(
				"invite cashback settled for %s: billing_source=%s rewarded_users=%d total_cashback=%d percent=%d",
				statDate,
				inviteCashbackBillingSource,
				rewardedUsers,
				totalCashback,
				common.InviteCashbackPercent,
			),
		)
	}
	return nil
}
