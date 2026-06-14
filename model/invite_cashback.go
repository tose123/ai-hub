package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const inviteCashbackWalletBillingSourceLike = `%"billing_source":"wallet"%`

type InviteCashbackRecord struct {
	Id              int    `json:"id"`
	StatDate        string `json:"stat_date" gorm:"type:varchar(10);uniqueIndex:idx_invite_cashback_stat_invitee,priority:1"`
	InviteeUserId   int    `json:"invitee_user_id" gorm:"column:invitee_user_id;uniqueIndex:idx_invite_cashback_stat_invitee,priority:2;index"`
	InviterUserId   int    `json:"inviter_user_id" gorm:"column:inviter_user_id;index"`
	ConsumedQuota   int    `json:"consumed_quota" gorm:"column:consumed_quota"`
	CashbackPercent int    `json:"cashback_percent" gorm:"column:cashback_percent"`
	CashbackQuota   int    `json:"cashback_quota" gorm:"column:cashback_quota"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
}

type InviteCashbackTaskState struct {
	TaskName          string `json:"task_name" gorm:"primaryKey;type:varchar(64);column:task_name"`
	LastCompletedDate string `json:"last_completed_date" gorm:"type:varchar(10);column:last_completed_date"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type InviteeRelation struct {
	Id        int `json:"id" gorm:"column:id"`
	InviterId int `json:"inviter_id" gorm:"column:inviter_id"`
}

func ListInviteeRelationsAfterID(afterID int, limit int) (users []InviteeRelation, err error) {
	if limit <= 0 {
		return users, nil
	}

	tx := DB.Unscoped().Model(&User{}).
		Select("id, inviter_id").
		Where("inviter_id > ?", 0)
	if afterID > 0 {
		tx = tx.Where("id > ?", afterID)
	}

	err = tx.Order("id asc").Limit(limit).Scan(&users).Error
	return users, err
}

func SumInviteeWalletNetQuota(userID int, startTimestamp int64, endTimestamp int64) (int, error) {
	if userID <= 0 {
		return 0, nil
	}

	var netQuota int
	err := LOG_DB.Table("logs").
		Select("COALESCE(SUM(CASE WHEN type = ? THEN quota WHEN type = ? THEN -quota ELSE 0 END), 0) AS net_quota", LogTypeConsume, LogTypeRefund).
		Where("user_id = ?", userID).
		Where("created_at >= ? AND created_at < ?", startTimestamp, endTimestamp).
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund}).
		Where("other LIKE ?", inviteCashbackWalletBillingSourceLike).
		Scan(&netQuota).Error
	if err != nil {
		return 0, err
	}
	return netQuota, nil
}

func GetOrCreateInviteCashbackTaskState(taskName string, initialLastCompletedDate string) (*InviteCashbackTaskState, bool, error) {
	if taskName == "" {
		return nil, false, errors.New("task name is empty")
	}

	var state InviteCashbackTaskState
	err := DB.Where("task_name = ?", taskName).First(&state).Error
	if err == nil {
		return &state, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	state = InviteCashbackTaskState{
		TaskName:          taskName,
		LastCompletedDate: initialLastCompletedDate,
		UpdatedAt:         common.GetTimestamp(),
	}
	if err = DB.Create(&state).Error; err != nil {
		return nil, false, err
	}
	return &state, true, nil
}

func UpdateInviteCashbackTaskState(taskName string, lastCompletedDate string) error {
	return DB.Model(&InviteCashbackTaskState{}).
		Where("task_name = ?", taskName).
		Updates(map[string]interface{}{
			"last_completed_date": lastCompletedDate,
			"updated_at":          common.GetTimestamp(),
		}).Error
}

func ApplyInviteCashback(statDate string, inviteeUserId int, inviterUserId int, consumedQuota int, cashbackPercent int, cashbackQuota int) (bool, error) {
	if statDate == "" {
		return false, errors.New("stat date is empty")
	}
	if inviteeUserId <= 0 || inviterUserId <= 0 {
		return false, errors.New("invalid invite relation")
	}
	if cashbackQuota <= 0 {
		return false, nil
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return false, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	record := InviteCashbackRecord{
		StatDate:        statDate,
		InviteeUserId:   inviteeUserId,
		InviterUserId:   inviterUserId,
		ConsumedQuota:   consumedQuota,
		CashbackPercent: cashbackPercent,
		CashbackQuota:   cashbackQuota,
	}
	res := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "stat_date"},
			{Name: "invitee_user_id"},
		},
		DoNothing: true,
	}).Create(&record)
	if res.Error != nil {
		tx.Rollback()
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		tx.Rollback()
		return false, nil
	}

	res = tx.Model(&User{}).
		Where("id = ?", inviterUserId).
		Updates(map[string]interface{}{
			"quota":       gorm.Expr("quota + ?", cashbackQuota),
			"aff_history": gorm.Expr("aff_history + ?", cashbackQuota),
		})
	if res.Error != nil {
		tx.Rollback()
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		tx.Rollback()
		return false, fmt.Errorf("inviter not found: %d", inviterUserId)
	}

	if LOG_DB == DB {
		if err := createInviteCashbackTopupLog(tx, inviterUserId, inviteeUserId, cashbackQuota); err != nil {
			tx.Rollback()
			return false, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return false, err
	}

	gopool.Go(func() {
		if err := cacheIncrUserQuota(inviterUserId, int64(cashbackQuota)); err != nil {
			common.SysLog("failed to increase inviter quota cache: " + err.Error())
		}
	})

	if LOG_DB != DB {
		RecordLog(
			inviterUserId,
			LogTypeTopup,
			fmt.Sprintf("邀请返现充值 %s，来自用户ID %d 的前一日余额消耗", logger.LogQuota(cashbackQuota), inviteeUserId),
		)
	}

	return true, nil
}

func createInviteCashbackTopupLog(tx *gorm.DB, inviterUserId int, inviteeUserId int, cashbackQuota int) error {
	username, _ := GetUsernameById(inviterUserId, false)
	log := &Log{
		UserId:    inviterUserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   fmt.Sprintf("邀请返现充值 %s，来自用户#%d 的前一日余额消耗", logger.LogQuota(cashbackQuota), inviteeUserId),
	}
	return tx.Create(log).Error
}
