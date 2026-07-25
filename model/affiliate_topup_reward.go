package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	AffiliateTopUpRewardStatusPending   = "pending"
	AffiliateTopUpRewardStatusCompleted = "completed"
)

const affiliateTopUpRewardDelaySeconds int64 = 24 * 60 * 60

type AffiliateTopUpReward struct {
	Id                int     `json:"id"`
	TradeNo           string  `json:"trade_no" gorm:"type:varchar(255);uniqueIndex"`
	TopUpId           int     `json:"topup_id" gorm:"index"`
	InviterId         int     `json:"inviter_id" gorm:"index"`
	InviteeId         int     `json:"invitee_id" gorm:"index"`
	TopUpQuota        int     `json:"topup_quota"`
	RewardQuota       int     `json:"reward_quota"`
	RewardRatio       float64 `json:"reward_ratio"`
	AffiliateRole     string  `json:"affiliate_role" gorm:"type:varchar(64);default:''"`
	AffiliateRoleName string  `json:"affiliate_role_name" gorm:"type:varchar(128);default:''"`
	PaymentProvider   string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	Status            string  `json:"status" gorm:"type:varchar(32);index"`
	AvailableAt       int64   `json:"available_at" gorm:"bigint;index"`
	RewardedAt        int64   `json:"rewarded_at" gorm:"bigint;default:0"`
	CreatedAt         int64   `json:"created_at" gorm:"bigint"`
	UpdatedAt         int64   `json:"updated_at" gorm:"bigint"`
}

type AffiliateInviteRecord struct {
	InviteeId            int    `json:"invitee_id"`
	Username             string `json:"username"`
	DisplayName          string `json:"display_name"`
	CreatedAt            int64  `json:"created_at"`
	CompletedRewardQuota int64  `json:"completed_reward_quota"`
	PendingRewardQuota   int64  `json:"pending_reward_quota"`
	RewardCount          int64  `json:"reward_count"`
}

type AffiliateInviteStats struct {
	InviteCount               int64 `json:"invite_count"`
	AvailableRewardQuota      int   `json:"available_reward_quota"`
	RegistrationRewardQuota   int   `json:"registration_reward_quota"`
	CompletedTopUpRewardQuota int64 `json:"completed_topup_reward_quota"`
	PendingTopUpRewardQuota   int64 `json:"pending_topup_reward_quota"`
	TotalRewardQuota          int64 `json:"total_reward_quota"`
}

type affiliateInviteRewardAggregate struct {
	InviteeId   int    `gorm:"column:invitee_id"`
	Status      string `gorm:"column:status"`
	RewardQuota int64  `gorm:"column:reward_quota"`
	RewardCount int64  `gorm:"column:reward_count"`
}

func ListAffiliateInvites(inviterId int, startIdx int, pageSize int) ([]AffiliateInviteRecord, int64, AffiliateInviteStats, error) {
	var stats AffiliateInviteStats
	var inviter User
	if err := DB.Select("aff_quota", "aff_history").Where("id = ?", inviterId).First(&inviter).Error; err != nil {
		return nil, 0, stats, err
	}
	stats.AvailableRewardQuota = inviter.AffQuota
	stats.RegistrationRewardQuota = inviter.AffHistoryQuota

	inviteQuery := DB.Model(&User{}).Where("inviter_id = ?", inviterId)
	if err := inviteQuery.Count(&stats.InviteCount).Error; err != nil {
		return nil, 0, stats, err
	}

	if err := DB.Model(&AffiliateTopUpReward{}).
		Where("inviter_id = ? AND status = ?", inviterId, AffiliateTopUpRewardStatusCompleted).
		Select("COALESCE(SUM(reward_quota), 0)").
		Scan(&stats.CompletedTopUpRewardQuota).Error; err != nil {
		return nil, 0, stats, err
	}
	if err := DB.Model(&AffiliateTopUpReward{}).
		Where("inviter_id = ? AND status = ?", inviterId, AffiliateTopUpRewardStatusPending).
		Select("COALESCE(SUM(reward_quota), 0)").
		Scan(&stats.PendingTopUpRewardQuota).Error; err != nil {
		return nil, 0, stats, err
	}
	stats.TotalRewardQuota = int64(stats.RegistrationRewardQuota) + stats.CompletedTopUpRewardQuota

	records := make([]AffiliateInviteRecord, 0)
	if err := inviteQuery.
		Select("id AS invitee_id", "username", "display_name", "created_at").
		Order("created_at DESC").
		Order("id DESC").
		Limit(pageSize).
		Offset(startIdx).
		Scan(&records).Error; err != nil {
		return nil, 0, stats, err
	}
	if len(records) == 0 {
		return records, stats.InviteCount, stats, nil
	}

	inviteeIds := make([]int, 0, len(records))
	for _, record := range records {
		inviteeIds = append(inviteeIds, record.InviteeId)
	}
	var aggregates []affiliateInviteRewardAggregate
	if err := DB.Model(&AffiliateTopUpReward{}).
		Select("invitee_id", "status", "SUM(reward_quota) AS reward_quota", "COUNT(*) AS reward_count").
		Where("inviter_id = ? AND invitee_id IN ?", inviterId, inviteeIds).
		Group("invitee_id, status").
		Scan(&aggregates).Error; err != nil {
		return nil, 0, stats, err
	}

	recordByInviteeId := make(map[int]*AffiliateInviteRecord, len(records))
	for i := range records {
		recordByInviteeId[records[i].InviteeId] = &records[i]
	}
	for _, aggregate := range aggregates {
		record := recordByInviteeId[aggregate.InviteeId]
		if record == nil {
			continue
		}
		record.RewardCount += aggregate.RewardCount
		switch aggregate.Status {
		case AffiliateTopUpRewardStatusCompleted:
			record.CompletedRewardQuota = aggregate.RewardQuota
		case AffiliateTopUpRewardStatusPending:
			record.PendingRewardQuota = aggregate.RewardQuota
		}
	}

	return records, stats.InviteCount, stats, nil
}

func CalculateAffiliateTopUpRewardQuota(topUpQuota int, ratio float64) int {
	if topUpQuota <= 0 || ratio <= 0 {
		return 0
	}
	return int(decimal.NewFromInt(int64(topUpQuota)).Mul(decimal.NewFromFloat(ratio)).IntPart())
}

func GetAffiliateRewardPolicyForUser(userId int) setting.AffiliateRewardPolicy {
	if userId <= 0 {
		return setting.ResolveAffiliateRewardPolicy("")
	}
	var user User
	if err := DB.Select("affiliate_role").Where("id = ?", userId).First(&user).Error; err != nil {
		return setting.ResolveAffiliateRewardPolicy("")
	}
	return setting.ResolveAffiliateRewardPolicy(user.AffiliateRole)
}

func CreateAffiliateTopUpReward(topUp *TopUp, topUpQuota int) (bool, error) {
	if topUp == nil || topUp.Id == 0 || topUp.TradeNo == "" {
		return false, nil
	}
	if topUpQuota <= 0 {
		return false, nil
	}

	completeTime := topUp.CompleteTime
	if completeTime == 0 {
		completeTime = common.GetTimestamp()
	}
	now := common.GetTimestamp()

	created := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var invitee User
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Select("id", "inviter_id").Where("id = ?", topUp.UserId).First(&invitee).Error; err != nil {
			return err
		}
		if invitee.InviterId == 0 || invitee.InviterId == invitee.Id {
			return nil
		}
		var inviter User
		if err := tx.Select("id", "affiliate_role").Where("id = ?", invitee.InviterId).First(&inviter).Error; err != nil {
			return err
		}
		policy := setting.ResolveAffiliateRewardPolicy(inviter.AffiliateRole)
		if policy.TopUpRewardRatio <= 0 {
			return nil
		}
		rewardQuota := CalculateAffiliateTopUpRewardQuota(topUpQuota, policy.TopUpRewardRatio)
		if rewardQuota <= 0 {
			return nil
		}
		if policy.TopUpRewardLimit > 0 {
			var rewardedCount int64
			if err := tx.Model(&AffiliateTopUpReward{}).
				Where("inviter_id = ? AND invitee_id = ?", invitee.InviterId, topUp.UserId).
				Count(&rewardedCount).Error; err != nil {
				return err
			}
			if rewardedCount >= int64(policy.TopUpRewardLimit) {
				return nil
			}
		}
		reward := &AffiliateTopUpReward{
			TradeNo:           topUp.TradeNo,
			TopUpId:           topUp.Id,
			InviterId:         invitee.InviterId,
			InviteeId:         topUp.UserId,
			TopUpQuota:        topUpQuota,
			RewardQuota:       rewardQuota,
			RewardRatio:       policy.TopUpRewardRatio,
			AffiliateRole:     policy.RoleId,
			AffiliateRoleName: policy.RoleName,
			PaymentProvider:   topUp.PaymentProvider,
			Status:            AffiliateTopUpRewardStatusPending,
			AvailableAt:       completeTime + affiliateTopUpRewardDelaySeconds,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := tx.Create(reward).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return false, nil
		}
		var existing AffiliateTopUpReward
		if findErr := DB.Select("id").Where("trade_no = ?", topUp.TradeNo).First(&existing).Error; findErr == nil {
			return false, nil
		}
		return false, err
	}
	return created, nil
}

func ProcessDueAffiliateTopUpRewards(limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	now := common.GetTimestamp()
	var rewards []AffiliateTopUpReward
	if err := DB.Where("status = ? AND available_at <= ?", AffiliateTopUpRewardStatusPending, now).
		Order("id asc").
		Limit(limit).
		Find(&rewards).Error; err != nil {
		return 0, err
	}
	processed := 0
	for _, reward := range rewards {
		ok, err := completeAffiliateTopUpReward(reward.Id)
		if err != nil {
			return processed, err
		}
		if ok {
			processed++
		}
	}
	return processed, nil
}

func completeAffiliateTopUpReward(rewardId int) (bool, error) {
	now := common.GetTimestamp()
	var logInviterId int
	var logInviteeId int
	var logRewardQuota int
	var logTopUpQuota int

	err := DB.Transaction(func(tx *gorm.DB) error {
		var reward AffiliateTopUpReward
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", rewardId).First(&reward).Error; err != nil {
			return err
		}
		if reward.Status != AffiliateTopUpRewardStatusPending || reward.AvailableAt > now {
			return nil
		}
		result := tx.Model(&AffiliateTopUpReward{}).Where("id = ? AND status = ?", reward.Id, AffiliateTopUpRewardStatusPending).
			Updates(map[string]interface{}{
				"status":      AffiliateTopUpRewardStatusCompleted,
				"rewarded_at": now,
				"updated_at":  now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Model(&User{}).Where("id = ?", reward.InviterId).Update("quota", gorm.Expr("quota + ?", reward.RewardQuota)).Error; err != nil {
			return err
		}
		logInviterId = reward.InviterId
		logInviteeId = reward.InviteeId
		logRewardQuota = reward.RewardQuota
		logTopUpQuota = reward.TopUpQuota
		return nil
	})
	if err != nil {
		return false, err
	}
	if logInviterId == 0 {
		return false, nil
	}
	RecordLog(logInviterId, LogTypeSystem, fmt.Sprintf(
		"邀请用户 %d 充值奖励到账，充值额度: %s，奖励额度: %s",
		logInviteeId,
		logger.LogQuota(logTopUpQuota),
		logger.LogQuota(logRewardQuota),
	))
	go func(userId int, rewardQuota int) {
		if err := cacheIncrUserQuota(userId, int64(rewardQuota)); err != nil {
			common.SysLog("failed to increase affiliate top-up reward quota cache: " + err.Error())
		}
	}(logInviterId, logRewardQuota)
	return true, nil
}
