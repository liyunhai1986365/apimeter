package service

import (
	"context"
	"fmt"
	"html"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	balanceForecastTickInterval = time.Hour
	balanceForecastFreshness    = 90 * time.Minute
	balanceForecastUserBatch    = 500

	balanceForecastLevelNotice   = "notice"
	balanceForecastLevelWarning  = "warning"
	balanceForecastLevelCritical = "critical"
)

var (
	balanceForecastOnce    sync.Once
	balanceForecastRunning atomic.Bool
)

func StartBalanceForecastTask() {
	balanceForecastOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("balance forecast task started: tick=%s", balanceForecastTickInterval))
			ticker := time.NewTicker(balanceForecastTickInterval)
			defer ticker.Stop()
			runBalanceForecastOnce(time.Now())
			for now := range ticker.C {
				runBalanceForecastOnce(now)
			}
		})
	})
}

func GetOrCalculateBalanceForecast(userId int) (*model.BalanceForecast, error) {
	forecast, err := model.GetBalanceForecastByUserId(userId)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if forecast != nil && now.Unix()-forecast.CalculatedAt <= int64(balanceForecastFreshness/time.Second) {
		return forecast, nil
	}
	return calculateBalanceForecastForUser(userId, now)
}

func calculateBalanceForecastForUser(userId int, now time.Time) (*model.BalanceForecast, error) {
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return nil, err
	}
	usage, err := model.GetWalletUsageAggregate(userId, now)
	if err != nil {
		return nil, err
	}
	return model.UpsertBalanceForecast(buildBalanceForecast(user.Id, user.Quota, usage, now))
}

func buildBalanceForecast(userId int, balance int, usage model.WalletUsageAggregate, now time.Time) *model.BalanceForecast {
	forecast := &model.BalanceForecast{
		UserId:          userId,
		Balance:         int64(balance),
		UsedLast24Hours: usage.Used24Hours,
		UsedLast7Days:   usage.Used7Days,
		SampleStartedAt: usage.FirstUsedAt,
		LastUsageAt:     usage.LastUsedAt,
		CalculatedAt:    now.Unix(),
		Status:          model.BalanceForecastStatusInsufficientData,
	}

	if balance <= 0 {
		forecast.Status = model.BalanceForecastStatusDepleted
	}
	if usage.Used24Hours <= 0 && usage.Used7Days <= 0 {
		return forecast
	}

	firstUsedAt := usage.FirstUsedAt
	if firstUsedAt <= 0 || firstUsedAt > now.Unix() {
		firstUsedAt = now.Unix()
	}
	hourWindowStart := max(firstUsedAt, now.Add(-24*time.Hour).Unix())
	dayWindowStart := max(firstUsedAt, now.Add(-7*24*time.Hour).Unix())
	hourSamples := math.Max(1, now.Sub(time.Unix(hourWindowStart, 0)).Hours())
	daySamples := math.Max(1, now.Sub(time.Unix(dayWindowStart, 0)).Hours()/24)
	hourlyConsumption := float64(usage.Used24Hours) / hourSamples
	dailyConsumption := float64(usage.Used7Days) / daySamples
	forecast.HourlyConsumption = int64(math.Round(hourlyConsumption))
	forecast.DailyConsumption = int64(math.Round(dailyConsumption))

	effectiveHourlyConsumption := math.Max(hourlyConsumption, dailyConsumption/24)
	if balance <= 0 || effectiveHourlyConsumption <= 0 {
		return forecast
	}

	forecast.Status = model.BalanceForecastStatusReady
	forecast.EstimatedHours = float64(balance) / effectiveHourlyConsumption
	// Extremely long projections are not meaningful and can overflow
	// time.Duration. Keep the duration, but omit the exhaustion timestamp.
	if forecast.EstimatedHours <= 100*365*24 {
		forecast.EstimatedExhaustedAt = now.Add(time.Duration(forecast.EstimatedHours * float64(time.Hour))).Unix()
	}
	return forecast
}

func runBalanceForecastOnce(now time.Time) {
	if !balanceForecastRunning.CompareAndSwap(false, true) {
		return
	}
	defer balanceForecastRunning.Store(false)

	aggregates, err := model.ListWalletUsageAggregates(now)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("balance forecast usage scan failed: %v", err))
		return
	}
	usageByUser := make(map[int]model.WalletUsageAggregate, len(aggregates))
	userIdSet := make(map[int]struct{}, len(aggregates))
	for _, aggregate := range aggregates {
		usageByUser[aggregate.UserId] = aggregate
		userIdSet[aggregate.UserId] = struct{}{}
	}
	existingUserIds, err := model.ListRecentBalanceForecastUserIds(now.Add(-8 * 24 * time.Hour).Unix())
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("balance forecast state scan failed: %v", err))
		return
	}
	for _, userId := range existingUserIds {
		userIdSet[userId] = struct{}{}
	}
	userIds := make([]int, 0, len(userIdSet))
	for userId := range userIdSet {
		userIds = append(userIds, userId)
	}

	for start := 0; start < len(userIds); start += balanceForecastUserBatch {
		end := min(start+balanceForecastUserBatch, len(userIds))
		users, err := model.ListBalanceForecastUsers(userIds[start:end])
		if err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("balance forecast user batch failed: %v", err))
			continue
		}
		for i := range users {
			user := &users[i]
			if user.Status != common.UserStatusEnabled {
				continue
			}
			forecast, err := model.UpsertBalanceForecast(buildBalanceForecast(user.Id, user.Quota, usageByUser[user.Id], now))
			if err != nil {
				logger.LogWarn(context.Background(), fmt.Sprintf("balance forecast update failed for user %d: %v", user.Id, err))
				continue
			}
			handleBalanceForecastReminder(user, forecast, now)
		}
	}
}

func balanceForecastNotificationLevel(estimatedHours float64) string {
	switch {
	case estimatedHours <= 0:
		return ""
	case estimatedHours <= 24:
		return balanceForecastLevelCritical
	case estimatedHours <= 72:
		return balanceForecastLevelWarning
	case estimatedHours <= 7*24:
		return balanceForecastLevelNotice
	default:
		return ""
	}
}

func balanceForecastNotificationSeverity(level string) int {
	switch level {
	case balanceForecastLevelNotice:
		return 1
	case balanceForecastLevelWarning:
		return 2
	case balanceForecastLevelCritical:
		return 3
	default:
		return 0
	}
}

func handleBalanceForecastReminder(user *model.User, forecast *model.BalanceForecast, now time.Time) {
	if user == nil || forecast == nil {
		return
	}
	if forecast.Status == model.BalanceForecastStatusInsufficientData {
		if forecast.LastNotifiedLevel != "" {
			_ = model.UpdateBalanceForecastNotificationState(user.Id, "", 0, forecast.Balance)
		}
		return
	}
	if forecast.Status != model.BalanceForecastStatusReady || forecast.Balance <= 0 {
		return
	}
	level := balanceForecastNotificationLevel(forecast.EstimatedHours)
	if forecast.LastNotifiedLevel != "" && forecast.LastNotifiedBalance > 0 && forecast.Balance > forecast.LastNotifiedBalance {
		// A top-up starts a new runway cycle. Adopt the current level as the
		// baseline so the task only sends again if the forecast worsens.
		_ = model.UpdateBalanceForecastNotificationState(user.Id, level, 0, forecast.Balance)
		return
	}
	if level == "" {
		if forecast.LastNotifiedLevel != "" {
			_ = model.UpdateBalanceForecastNotificationState(user.Id, "", 0, forecast.Balance)
		}
		return
	}
	if balanceForecastNotificationSeverity(level) <= balanceForecastNotificationSeverity(forecast.LastNotifiedLevel) {
		return
	}

	userSetting := user.GetSetting()
	if strings.TrimSpace(userSetting.NotificationEmail) == "" && strings.TrimSpace(user.Email) == "" {
		return
	}
	userSetting.NotifyType = dto.NotifyTypeEmail
	subject, content := balanceForecastEmail(level, forecast, userSetting.Language)
	err := NotifyUser(
		user.Id,
		user.Email,
		userSetting,
		dto.NewNotify(dto.NotifyTypeBalanceForecast+"_"+level, subject, content, nil),
	)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("balance forecast reminder failed for user %d: %v", user.Id, err))
		return
	}
	if err := model.UpdateBalanceForecastNotificationState(user.Id, level, now.Unix(), forecast.Balance); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("balance forecast reminder state failed for user %d: %v", user.Id, err))
	}
}

func balanceForecastEmail(level string, forecast *model.BalanceForecast, language string) (string, string) {
	isChinese := strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh")
	duration := formatBalanceForecastDuration(forecast.EstimatedHours, isChinese)
	exhaustedAt := "-"
	if forecast.EstimatedExhaustedAt > 0 {
		exhaustedAt = time.Unix(forecast.EstimatedExhaustedAt, 0).UTC().Format("2006-01-02 15:04 UTC")
	}
	topUpLink := html.EscapeString(PaymentReturnURL("/console/topup"))
	balance := html.EscapeString(logger.FormatQuota(int(forecast.Balance)))
	hourly := html.EscapeString(logger.FormatQuota(int(forecast.HourlyConsumption)))
	daily := html.EscapeString(logger.FormatQuota(int(forecast.DailyConsumption)))

	if isChinese {
		threshold := map[string]string{
			balanceForecastLevelNotice:   "7 天",
			balanceForecastLevelWarning:  "3 天",
			balanceForecastLevelCritical: "24 小时",
		}[level]
		subject := fmt.Sprintf("余额预计可用不足 %s，请及时充值", threshold)
		content := fmt.Sprintf(
			"<p>根据您近期的钱包消耗趋势，当前余额预计还可使用 <strong>%s</strong>。</p>"+
				"<ul><li>当前余额：%s</li><li>近 24 小时平均每小时消耗：%s</li><li>近 7 天平均每日消耗：%s</li><li>预计耗尽时间：%s</li></ul>"+
				"<p><a href='%s'>立即充值</a></p>"+
				"<p>该时间根据近期实际消耗动态估算，使用量变化会影响结果。</p>",
			duration, balance, hourly, daily, exhaustedAt, topUpLink,
		)
		return subject, content
	}

	threshold := map[string]string{
		balanceForecastLevelNotice:   "7 days",
		balanceForecastLevelWarning:  "3 days",
		balanceForecastLevelCritical: "24 hours",
	}[level]
	subject := fmt.Sprintf("Estimated balance runway is under %s", threshold)
	content := fmt.Sprintf(
		"<p>Based on your recent wallet usage, your current balance is estimated to last <strong>%s</strong>.</p>"+
			"<ul><li>Current balance: %s</li><li>Average hourly usage over the last 24 hours: %s</li><li>Average daily usage over the last 7 days: %s</li><li>Estimated depletion time: %s</li></ul>"+
			"<p><a href='%s'>Top up now</a></p>"+
			"<p>This estimate updates with your recent usage and may change as usage changes.</p>",
		duration, balance, hourly, daily, exhaustedAt, topUpLink,
	)
	return subject, content
}

func formatBalanceForecastDuration(hours float64, chinese bool) string {
	if hours < 24 {
		if chinese {
			return fmt.Sprintf("约 %.0f 小时", math.Max(1, math.Ceil(hours)))
		}
		return fmt.Sprintf("about %.0f hours", math.Max(1, math.Ceil(hours)))
	}
	days := hours / 24
	if chinese {
		return fmt.Sprintf("约 %.1f 天", days)
	}
	return fmt.Sprintf("about %.1f days", days)
}
