package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	BalanceForecastStatusReady            = "ready"
	BalanceForecastStatusDepleted         = "depleted"
	BalanceForecastStatusInsufficientData = "insufficient_data"
)

// BalanceForecast stores the latest wallet runway calculation for a user.
// Notification state is kept alongside the forecast so repeated task runs do
// not send the same reminder more than once.
type BalanceForecast struct {
	Id                   int     `json:"id"`
	UserId               int     `json:"user_id" gorm:"uniqueIndex"`
	Balance              int64   `json:"balance" gorm:"type:bigint;default:0"`
	HourlyConsumption    int64   `json:"hourly_consumption" gorm:"type:bigint;default:0"`
	DailyConsumption     int64   `json:"daily_consumption" gorm:"type:bigint;default:0"`
	UsedLast24Hours      int64   `json:"used_last_24_hours" gorm:"column:used_last_24_hours;type:bigint;default:0"`
	UsedLast7Days        int64   `json:"used_last_7_days" gorm:"column:used_last_7_days;type:bigint;default:0"`
	EstimatedHours       float64 `json:"estimated_hours" gorm:"default:0"`
	EstimatedExhaustedAt int64   `json:"estimated_exhausted_at" gorm:"bigint;default:0"`
	SampleStartedAt      int64   `json:"sample_started_at" gorm:"bigint;default:0"`
	LastUsageAt          int64   `json:"last_usage_at" gorm:"bigint;index"`
	CalculatedAt         int64   `json:"calculated_at" gorm:"bigint;index"`
	Status               string  `json:"status" gorm:"type:varchar(32);index"`
	LastNotifiedLevel    string  `json:"-" gorm:"type:varchar(16);default:''"`
	LastNotifiedAt       int64   `json:"-" gorm:"bigint;default:0"`
	LastNotifiedBalance  int64   `json:"-" gorm:"type:bigint;default:0"`
	CreatedAt            int64   `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt            int64   `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type WalletUsageAggregate struct {
	UserId      int   `json:"user_id"`
	Used24Hours int64 `json:"used_24_hours" gorm:"column:used_24_hours"`
	Used7Days   int64 `json:"used_7_days" gorm:"column:used_7_days"`
	FirstUsedAt int64 `json:"first_used_at"`
	LastUsedAt  int64 `json:"last_used_at"`
}

func walletUsageAggregateQuery(now time.Time) *gorm.DB {
	end := now.Unix()
	start24Hours := now.Add(-24 * time.Hour).Unix()
	start7Days := now.Add(-7 * 24 * time.Hour).Unix()
	return LOG_DB.Model(&BillingUsageItem{}).
		Select(
			"user_id, "+
				"COALESCE(SUM(CASE WHEN billed_at >= ? THEN settlement_amount ELSE 0 END), 0) AS used_24_hours, "+
				"COALESCE(SUM(settlement_amount), 0) AS used_7_days, "+
				"MIN(billed_at) AS first_used_at, "+
				"MAX(billed_at) AS last_used_at",
			start24Hours,
		).
		Where("billing_source = ?", BillingSourceWallet).
		Where("settlement_amount > 0").
		Where("billed_at >= ? AND billed_at <= ?", start7Days, end)
}

func GetWalletUsageAggregate(userId int, now time.Time) (WalletUsageAggregate, error) {
	aggregate := WalletUsageAggregate{UserId: userId}
	if userId <= 0 {
		return aggregate, nil
	}
	err := walletUsageAggregateQuery(now).
		Where("user_id = ?", userId).
		Group("user_id").
		Scan(&aggregate).Error
	return aggregate, err
}

func ListWalletUsageAggregates(now time.Time) ([]WalletUsageAggregate, error) {
	aggregates := make([]WalletUsageAggregate, 0)
	err := walletUsageAggregateQuery(now).
		Group("user_id").
		Scan(&aggregates).Error
	return aggregates, err
}

func GetBalanceForecastByUserId(userId int) (*BalanceForecast, error) {
	forecast := &BalanceForecast{}
	err := DB.Where("user_id = ?", userId).First(forecast).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return forecast, err
}

func UpsertBalanceForecast(forecast *BalanceForecast) (*BalanceForecast, error) {
	if forecast == nil || forecast.UserId <= 0 {
		return nil, errors.New("invalid balance forecast")
	}
	stored := &BalanceForecast{}
	metrics := map[string]interface{}{
		"balance":                forecast.Balance,
		"hourly_consumption":     forecast.HourlyConsumption,
		"daily_consumption":      forecast.DailyConsumption,
		"used_last_24_hours":     forecast.UsedLast24Hours,
		"used_last_7_days":       forecast.UsedLast7Days,
		"estimated_hours":        forecast.EstimatedHours,
		"estimated_exhausted_at": forecast.EstimatedExhaustedAt,
		"sample_started_at":      forecast.SampleStartedAt,
		"last_usage_at":          forecast.LastUsageAt,
		"calculated_at":          forecast.CalculatedAt,
		"status":                 forecast.Status,
	}
	err := DB.Where("user_id = ?", forecast.UserId).
		Assign(metrics).
		FirstOrCreate(stored, BalanceForecast{UserId: forecast.UserId}).Error
	return stored, err
}

func ListRecentBalanceForecastUserIds(minLastUsageAt int64) ([]int, error) {
	var userIds []int
	err := DB.Model(&BalanceForecast{}).
		Where("last_usage_at >= ?", minLastUsageAt).
		Pluck("user_id", &userIds).Error
	return userIds, err
}

func ListBalanceForecastUsers(userIds []int) ([]User, error) {
	if len(userIds) == 0 {
		return []User{}, nil
	}
	var users []User
	err := DB.
		Select("id", "quota", "email", "setting", "status").
		Where("id IN ?", userIds).
		Find(&users).Error
	return users, err
}

func UpdateBalanceForecastNotificationState(userId int, level string, notifiedAt int64, balance int64) error {
	return DB.Model(&BalanceForecast{}).
		Where("user_id = ?", userId).
		Updates(map[string]interface{}{
			"last_notified_level":   level,
			"last_notified_at":      notifiedAt,
			"last_notified_balance": balance,
		}).Error
}
