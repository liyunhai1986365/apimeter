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
	workspaceQuotaResetTickInterval = 1 * time.Minute
	workspaceQuotaResetBatchSize    = 300
)

var (
	workspaceQuotaResetOnce    sync.Once
	workspaceQuotaResetRunning atomic.Bool
)

func StartWorkspaceQuotaResetTask() {
	workspaceQuotaResetOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("workspace quota reset task started: tick=%s", workspaceQuotaResetTickInterval))
			ticker := time.NewTicker(workspaceQuotaResetTickInterval)
			defer ticker.Stop()

			runWorkspaceQuotaResetOnce()
			for range ticker.C {
				runWorkspaceQuotaResetOnce()
			}
		})
	})
}

func runWorkspaceQuotaResetOnce() {
	if !workspaceQuotaResetRunning.CompareAndSwap(false, true) {
		return
	}
	defer workspaceQuotaResetRunning.Store(false)

	ctx := context.Background()
	totalReset := 0
	for {
		n, err := model.ResetDueWorkspaceQuotaConfigs(workspaceQuotaResetBatchSize, time.Time{})
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("workspace quota reset task failed: %v", err))
			return
		}
		if n == 0 {
			break
		}
		totalReset += n
		if n < workspaceQuotaResetBatchSize {
			break
		}
	}
	if common.DebugEnabled && totalReset > 0 {
		logger.LogDebug(ctx, "workspace quota reset task: reset_count=%d", totalReset)
	}
}
