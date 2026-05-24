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
	affiliateTopUpRewardTickInterval = 1 * time.Minute
	affiliateTopUpRewardBatchSize    = 200
)

var (
	affiliateTopUpRewardOnce    sync.Once
	affiliateTopUpRewardRunning atomic.Bool
)

func StartAffiliateTopUpRewardTask() {
	affiliateTopUpRewardOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("affiliate topup reward task started: tick=%s", affiliateTopUpRewardTickInterval))
			ticker := time.NewTicker(affiliateTopUpRewardTickInterval)
			defer ticker.Stop()

			runAffiliateTopUpRewardOnce()
			for range ticker.C {
				runAffiliateTopUpRewardOnce()
			}
		})
	})
}

func runAffiliateTopUpRewardOnce() {
	if !affiliateTopUpRewardRunning.CompareAndSwap(false, true) {
		return
	}
	defer affiliateTopUpRewardRunning.Store(false)

	ctx := context.Background()
	total := 0
	for {
		n, err := model.ProcessDueAffiliateTopUpRewards(affiliateTopUpRewardBatchSize)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("affiliate topup reward task failed: %v", err))
			return
		}
		if n == 0 {
			break
		}
		total += n
		if n < affiliateTopUpRewardBatchSize {
			break
		}
	}
	if common.DebugEnabled && total > 0 {
		logger.LogDebug(ctx, "affiliate topup reward task processed: count=%d", total)
	}
}
