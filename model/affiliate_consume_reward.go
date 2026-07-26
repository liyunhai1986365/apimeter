package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type AffiliateConsumeReward struct {
	Id                 int     `json:"id"`
	PeriodStart        int64   `json:"period_start" gorm:"bigint;uniqueIndex:idx_affiliate_consume_reward_period,priority:1;index"`
	PeriodEnd          int64   `json:"period_end" gorm:"bigint"`
	InviterId          int     `json:"inviter_id" gorm:"index"`
	InviteeId          int     `json:"invitee_id" gorm:"uniqueIndex:idx_affiliate_consume_reward_period,priority:2;index"`
	ConsumeQuota       int64   `json:"consume_quota"`
	RewardQuota        int64   `json:"reward_quota"`
	RewardRatio        float64 `json:"reward_ratio"`
	AffiliateRole      string  `json:"affiliate_role" gorm:"type:varchar(64);default:''"`
	AffiliateRoleName  string  `json:"affiliate_role_name" gorm:"type:varchar(128);default:''"`
	AffHistoryCredited bool    `json:"aff_history_credited" gorm:"default:false;index"`
	RewardedAt         int64   `json:"rewarded_at" gorm:"bigint;index"`
	CreatedAt          int64   `json:"created_at" gorm:"bigint"`
	UpdatedAt          int64   `json:"updated_at" gorm:"bigint"`
}

type affiliateInviteeLink struct {
	InviteeId int `gorm:"column:invitee_id"`
	InviterId int `gorm:"column:inviter_id"`
}

type affiliateConsumeAggregate struct {
	UserId       int   `gorm:"column:user_id"`
	ConsumeQuota int64 `gorm:"column:consume_quota"`
}

func CalculateAffiliateConsumeRewardQuota(consumeQuota int64, ratio float64) int64 {
	if consumeQuota <= 0 || ratio <= 0 {
		return 0
	}
	return decimal.NewFromInt(consumeQuota).Mul(decimal.NewFromFloat(ratio)).IntPart()
}

// ProcessAffiliateConsumeRewards credits rewards for invitees' net usage in
// the half-open interval [periodStart, periodEnd). The period and invitee form
// an idempotency key, so retries and concurrent task runs cannot double-credit.
func ProcessAffiliateConsumeRewards(periodStart int64, periodEnd int64, batchSize int) (int, error) {
	if periodStart <= 0 || periodEnd <= periodStart {
		return 0, fmt.Errorf("invalid affiliate consume reward period")
	}
	if LOG_DB == nil {
		return 0, fmt.Errorf("log database is not initialized")
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	if !setting.AffiliateConsumeRewardsEnabled() {
		return 0, nil
	}

	processed := 0
	lastInviteeId := 0
	for {
		var links []affiliateInviteeLink
		if err := DB.Model(&User{}).
			Select("id AS invitee_id, inviter_id").
			Where("id > ? AND inviter_id > 0", lastInviteeId).
			Order("id ASC").
			Limit(batchSize).
			Scan(&links).Error; err != nil {
			return processed, err
		}
		if len(links) == 0 {
			break
		}

		inviteeIds := make([]int, 0, len(links))
		linkByInviteeId := make(map[int]affiliateInviteeLink, len(links))
		for _, link := range links {
			inviteeIds = append(inviteeIds, link.InviteeId)
			linkByInviteeId[link.InviteeId] = link
			lastInviteeId = link.InviteeId
		}

		var aggregates []affiliateConsumeAggregate
		if err := LOG_DB.Model(&Log{}).
			Select("user_id, SUM(CASE WHEN type = ? THEN -quota ELSE quota END) AS consume_quota", LogTypeRefund).
			Where("user_id IN ? AND created_at >= ? AND created_at < ? AND type IN ?", inviteeIds, periodStart, periodEnd, []int{LogTypeConsume, LogTypeRefund}).
			Group("user_id").
			Scan(&aggregates).Error; err != nil {
			return processed, err
		}

		for _, aggregate := range aggregates {
			if aggregate.ConsumeQuota <= 0 {
				continue
			}
			link, ok := linkByInviteeId[aggregate.UserId]
			if !ok || link.InviterId == aggregate.UserId {
				continue
			}
			created, err := createAffiliateConsumeReward(aggregate.UserId, aggregate.ConsumeQuota, periodStart, periodEnd)
			if err != nil {
				return processed, err
			}
			if created {
				processed++
			}
		}

		if len(links) < batchSize {
			break
		}
	}
	return processed, nil
}

func createAffiliateConsumeReward(inviteeId int, consumeQuota int64, periodStart int64, periodEnd int64) (bool, error) {
	now := common.GetTimestamp()
	var completed *AffiliateConsumeReward

	err := DB.Transaction(func(tx *gorm.DB) error {
		var invitee User
		if err := tx.Select("id", "inviter_id").Where("id = ?", inviteeId).First(&invitee).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if invitee.InviterId <= 0 || invitee.InviterId == invitee.Id {
			return nil
		}

		var inviter User
		if err := tx.Select("id", "affiliate_role").Where("id = ?", invitee.InviterId).First(&inviter).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		policy := setting.ResolveAffiliateRewardPolicy(inviter.AffiliateRole)
		rewardQuota := CalculateAffiliateConsumeRewardQuota(consumeQuota, policy.ConsumeRewardRatio)
		if rewardQuota <= 0 {
			return nil
		}

		reward := &AffiliateConsumeReward{
			PeriodStart:        periodStart,
			PeriodEnd:          periodEnd,
			InviterId:          inviter.Id,
			InviteeId:          invitee.Id,
			ConsumeQuota:       consumeQuota,
			RewardQuota:        rewardQuota,
			RewardRatio:        policy.ConsumeRewardRatio,
			AffiliateRole:      policy.RoleId,
			AffiliateRoleName:  policy.RoleName,
			AffHistoryCredited: true,
			RewardedAt:         now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := tx.Create(reward).Error; err != nil {
			return err
		}
		if err := creditAffiliateRewardAccount(tx, inviter.Id, rewardQuota); err != nil {
			return err
		}
		completed = reward
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return false, nil
		}
		var existing AffiliateConsumeReward
		if findErr := DB.Select("id").Where("period_start = ? AND invitee_id = ?", periodStart, inviteeId).First(&existing).Error; findErr == nil {
			return false, nil
		}
		return false, err
	}
	if completed == nil {
		return false, nil
	}

	RecordLog(completed.InviterId, LogTypeSystem, fmt.Sprintf(
		"邀请用户 %d 的 %s 实际消耗奖励到账，消耗额度: %s，奖励额度: %s",
		completed.InviteeId,
		time.Unix(completed.PeriodStart, 0).In(time.Local).Format("2006-01-02"),
		logger.LogQuota(int(completed.ConsumeQuota)),
		logger.LogQuota(int(completed.RewardQuota)),
	))
	go func(userId int) {
		if err := InvalidateUserCache(userId); err != nil {
			common.SysLog("failed to invalidate affiliate consume reward user cache: " + err.Error())
		}
	}(completed.InviterId)
	return true, nil
}
