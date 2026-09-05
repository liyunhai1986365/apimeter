package service

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	SmartMaxGroupRetries = 3
	SmartMaxAttempts     = 12
	SmartRetryWindow     = 2 * time.Minute
	initialRetryParamKey = "relay_initial_retry_param"
	smartRetryStateKey   = "relay_smart_retry_state"
)

// This state belongs to one client request, including its fallback model. It
// never stores raw credentials or survives into another request.
type smartRetryState struct {
	started        time.Time
	attempts       int
	failedKeys     map[[32]byte]bool
	failedChannels map[int]bool
	groupAttempts  map[string]int
}

func requestSmartRetryState(c *gin.Context) *smartRetryState {
	if c == nil || !IsRoutingStrategyTokenPolicy(c) {
		return nil
	}
	if value, ok := c.Get(smartRetryStateKey); ok {
		return value.(*smartRetryState)
	}
	started := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if started.IsZero() {
		started = time.Now()
	}
	state := &smartRetryState{started: started, failedKeys: make(map[[32]byte]bool), failedChannels: make(map[int]bool), groupAttempts: make(map[string]int)}
	c.Set(smartRetryStateKey, state)
	return state
}

func (p *RetryParam) MaxGroupRetries() int {
	limit := max(0, common.RetryTimes)
	if IsRoutingStrategyTokenPolicy(p.Ctx) {
		limit = min(limit, SmartMaxGroupRetries)
	}
	return limit
}

func SmartRetryBudgetAvailable(c *gin.Context) bool {
	state := requestSmartRetryState(c)
	return state == nil || (state.attempts < SmartMaxAttempts && (state.attempts == 0 || time.Since(state.started) < SmartRetryWindow))
}

// BeginAttempt applies a request-wide cap without cancelling a healthy stream.
// The time window limits starting another attempt, not the length of a response.
func (p *RetryParam) BeginAttempt() bool {
	if !SmartRetryBudgetAvailable(p.Ctx) {
		return false
	}
	if state := requestSmartRetryState(p.Ctx); state != nil {
		p.attempt, p.attemptInitialized = state.attempts, true
		state.attempts++
	}
	p.Ctx.Set("relay_retry_index", p.GetAttempt())
	return true
}

func WaitBeforeSmartRetry(c *gin.Context) bool {
	state := requestSmartRetryState(c)
	if state == nil {
		return true
	}
	if !SmartRetryBudgetAvailable(c) {
		return false
	}
	delay := time.Duration(100*(1<<min(max(state.attempts-1, 0), 3))+rand.Intn(101)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	if c.Request == nil {
		<-timer.C
		return SmartRetryBudgetAvailable(c)
	}
	select {
	case <-c.Request.Context().Done():
		return false
	case <-timer.C:
		return SmartRetryBudgetAvailable(c)
	}
}

// AdoptInitialSelection connects the distributor's selection to the first
// controller attempt, including a pending switch when the group budget is zero.
func (p *RetryParam) AdoptInitialSelection() {
	if raw, ok := p.Ctx.Get(initialRetryParamKey); ok {
		initial := raw.(RetryParam)
		if initial.TokenGroup == p.TokenGroup && initial.ModelName == p.ModelName {
			p.SetRetry(initial.GetRetry())
			p.resetNextTry = initial.resetNextTry
			p.nextTokenGroupAvailable = initial.nextTokenGroupAvailable
			p.currentTokenGroupIndex = initial.currentTokenGroupIndex
			p.tokenGroupCount = initial.tokenGroupCount
			p.tokenGroupFailover = initial.tokenGroupFailover
			return
		}
	}
	// Affinity and explicitly selected channels may bypass the random selector.
	groups := ResolveTokenGroupChain(p.Ctx, p.TokenGroup)
	if len(groups) == 0 && p.TokenGroup == AutoGroupName && !IsRoutingStrategyTokenPolicy(p.Ctx) {
		groups = GetRequestAutoGroups(p.Ctx, common.GetContextKeyString(p.Ctx, constant.ContextKeyUserGroup))
	}
	p.currentTokenGroupIndex = -1
	p.tokenGroupCount = len(groups)
	p.tokenGroupFailover = tokenGroupFailoverEnabled(p.Ctx, groups, IsRoutingStrategyTokenPolicy(p.Ctx))
	selected := common.GetContextKeyString(p.Ctx, constant.ContextKeyAutoGroup)
	for i, group := range groups {
		if group == selected {
			p.currentTokenGroupIndex = i
			break
		}
	}
	if p.GetRetry() >= p.MaxGroupRetries() {
		p.AdvanceToNextTokenGroup()
	}
}

func channelRetryFingerprint(channel *model.Channel, modelName, key string) [32]byte {
	value := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	// Keep distinct model mappings, organizations and request overrides eligible:
	// the same credential can legitimately address a different upstream target.
	identity := []string{fmt.Sprint(channel.Type), strings.TrimRight(channel.GetBaseURL(), "/"), channel.Other,
		value(channel.OpenAIOrganization), value(channel.ModelMapping), value(channel.ParamOverride),
		value(channel.HeaderOverride), value(channel.Setting), channel.OtherSettings, modelName, key}
	return sha256.Sum256([]byte(strings.Join(identity, "\x00")))
}

func ChannelRetryKeyAllowed(c *gin.Context, channel *model.Channel, modelName, key string) bool {
	state := requestSmartRetryState(c)
	return state == nil || (!state.failedChannels[channel.Id] && !state.failedKeys[channelRetryFingerprint(channel, modelName, key)])
}

func SmartRetryChannelFilter(c *gin.Context, modelName string) model.ChannelFilter {
	if requestSmartRetryState(c) == nil {
		return nil
	}
	return func(channel *model.Channel) bool {
		if channel == nil {
			return false
		}
		if !channel.ChannelInfo.IsMultiKey {
			return ChannelRetryKeyAllowed(c, channel, modelName, channel.Key)
		}
		for i, key := range channel.GetKeys() {
			status, exists := channel.ChannelInfo.MultiKeyStatusList[i]
			if (!exists || status == common.ChannelStatusEnabled) && ChannelRetryKeyAllowed(c, channel, modelName, key) {
				return true
			}
		}
		return false
	}
}

func MarkSmartRetryChannelFailure(c *gin.Context, channel *model.Channel, modelName string, keyUnavailable bool) {
	state := requestSmartRetryState(c)
	if state == nil || channel == nil {
		return
	}
	if keyUnavailable {
		state.failedChannels[channel.Id] = true
		return
	}
	state.failedKeys[channelRetryFingerprint(channel, modelName, common.GetContextKeyString(c, constant.ContextKeyChannelKey))] = true
}

func SmartRetryGroupAvailable(c *gin.Context, group string) bool {
	state := requestSmartRetryState(c)
	return state == nil || state.groupAttempts[group] < min(max(common.RetryTimes, 0), SmartMaxGroupRetries)+1
}

func RecordSmartRetryGroupAttempt(c *gin.Context, group string) {
	if state := requestSmartRetryState(c); state != nil {
		state.groupAttempts[group]++
	}
}

func SkipSmartRetryGroup(c *gin.Context, group string) {
	if state := requestSmartRetryState(c); state != nil {
		state.groupAttempts[group] = SmartMaxGroupRetries + 1
	}
}

func AppendSmartRetryAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	state := requestSmartRetryState(c)
	if state == nil || adminInfo == nil {
		return
	}
	groups := make(map[string]int, len(state.groupAttempts))
	for group, count := range state.groupAttempts {
		groups[group] = count
	}
	adminInfo["smart_retry"] = map[string]interface{}{
		"attempts":          state.attempts,
		"max_attempts":      SmartMaxAttempts,
		"max_group_retries": min(max(common.RetryTimes, 0), SmartMaxGroupRetries),
		"elapsed_ms":        time.Since(state.started).Milliseconds(),
		"retry_window_ms":   SmartRetryWindow.Milliseconds(),
		"group_attempts":    groups,
	}
}
