package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	affiliateConsumeRewardTickInterval = time.Hour
	affiliateConsumeRewardBatchSize    = 500
)

var (
	affiliateConsumeRewardOnce    sync.Once
	affiliateConsumeRewardRunning atomic.Bool
)

func StartAffiliateConsumeRewardTask() {
	affiliateConsumeRewardOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("affiliate consume reward task started: tick=%s", affiliateConsumeRewardTickInterval))
			ticker := time.NewTicker(affiliateConsumeRewardTickInterval)
			defer ticker.Stop()

			runAffiliateConsumeRewardOnce(time.Now())
			for now := range ticker.C {
				runAffiliateConsumeRewardOnce(now)
			}
		})
	})
}

func runAffiliateConsumeRewardOnce(now time.Time) {
	if !affiliateConsumeRewardRunning.CompareAndSwap(false, true) {
		return
	}
	defer affiliateConsumeRewardRunning.Store(false)

	localNow := now.In(time.Local)
	periodEnd := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	periodStart := periodEnd.AddDate(0, 0, -1)
	processed, err := model.ProcessAffiliateConsumeRewards(periodStart.Unix(), periodEnd.Unix(), affiliateConsumeRewardBatchSize)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("affiliate consume reward task failed: %v", err))
		return
	}
	if common.DebugEnabled && processed > 0 {
		logger.LogDebug(context.Background(), "affiliate consume reward task processed: period=%s count=%d", periodStart.Format("2006-01-02"), processed)
	}
}
