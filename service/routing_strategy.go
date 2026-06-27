package service

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/routing_strategy_setting"
)

type RoutingStrategyScore struct {
	Group        string  `json:"group"`
	Rank         int     `json:"rank"`
	GroupRatio   float64 `json:"group_ratio"`
	SuccessRate  float64 `json:"success_rate"`
	LatencyMs    float64 `json:"latency_ms"`
	RequestCount int64   `json:"request_count"`
	Score        float64 `json:"score"`
}

type RoutingStrategyRefreshResult struct {
	UpdatedSnapshots int `json:"updated_snapshots"`
	UserGroups       int `json:"user_groups"`
}

func ResolveRoutingStrategyGroups(strategy string, userGroup string, fallback []string) []string {
	strategy = strings.TrimSpace(strategy)
	userGroup = strings.TrimSpace(userGroup)
	if !model.ValidRoutingStrategy(strategy) || userGroup == "" {
		return fallback
	}
	snapshot, err := model.GetRoutingStrategySnapshot(strategy, userGroup)
	if err != nil || snapshot == nil || strings.TrimSpace(snapshot.Groups) == "" {
		return fallback
	}
	var groups []string
	if err := common.UnmarshalJsonStr(snapshot.Groups, &groups); err != nil {
		return fallback
	}

	allowed := make(map[string]bool, len(fallback))
	for _, group := range fallback {
		group = strings.TrimSpace(group)
		if group != "" {
			allowed[group] = true
		}
	}
	result := make([]string, 0, len(groups))
	seen := map[string]bool{}
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] || !allowed[group] {
			continue
		}
		seen[group] = true
		result = append(result, group)
	}
	for _, group := range fallback {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		result = append(result, group)
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func RefreshRoutingStrategySnapshots(ctx context.Context) (RoutingStrategyRefreshResult, error) {
	cfg := routing_strategy_setting.GetSetting()
	userGroups := routingStrategyUserGroups()
	if len(userGroups) == 0 {
		userGroups = []string{"default"}
	}

	perfByGroup := map[string]perfmetrics.GroupSummary{}
	if result, err := perfmetrics.QueryGroupSummary(routing_strategy_setting.GetWindowHours(), nil); err == nil {
		for _, group := range result.Groups {
			perfByGroup[group.Group] = group
		}
	}

	updated := 0
	for _, userGroup := range userGroups {
		select {
		case <-ctx.Done():
			return RoutingStrategyRefreshResult{UpdatedSnapshots: updated, UserGroups: len(userGroups)}, ctx.Err()
		default:
		}
		groups := GetUserAutoGroup(userGroup)
		if len(groups) == 0 {
			groups = routingStrategySortedGroupNames(GetUserUsableGroups(userGroup))
		}
		for _, strategy := range model.RoutingStrategies() {
			manualOverride, err := model.RoutingStrategySnapshotHasManualOverride(strategy, userGroup)
			if err != nil {
				return RoutingStrategyRefreshResult{UpdatedSnapshots: updated, UserGroups: len(userGroups)}, err
			}
			if manualOverride {
				continue
			}
			scores := buildRoutingStrategyScores(strategy, groups, perfByGroup, cfg)
			groupNames := make([]string, 0, len(scores))
			scoreMap := make(map[string]RoutingStrategyScore, len(scores))
			for i := range scores {
				scores[i].Rank = i + 1
				groupNames = append(groupNames, scores[i].Group)
				scoreMap[scores[i].Group] = scores[i]
			}
			groupsJSON, err := common.Marshal(groupNames)
			if err != nil {
				return RoutingStrategyRefreshResult{UpdatedSnapshots: updated, UserGroups: len(userGroups)}, err
			}
			scoresJSON, err := common.Marshal(scoreMap)
			if err != nil {
				return RoutingStrategyRefreshResult{UpdatedSnapshots: updated, UserGroups: len(userGroups)}, err
			}
			configJSON, err := common.Marshal(cfg)
			if err != nil {
				return RoutingStrategyRefreshResult{UpdatedSnapshots: updated, UserGroups: len(userGroups)}, err
			}
			if err := model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{
				Strategy:  strategy,
				UserGroup: userGroup,
				Groups:    string(groupsJSON),
				Scores:    string(scoresJSON),
				Config:    string(configJSON),
			}); err != nil {
				return RoutingStrategyRefreshResult{UpdatedSnapshots: updated, UserGroups: len(userGroups)}, err
			}
			updated++
		}
	}
	return RoutingStrategyRefreshResult{UpdatedSnapshots: updated, UserGroups: len(userGroups)}, nil
}

func buildRoutingStrategyScores(strategy string, groups []string, perfByGroup map[string]perfmetrics.GroupSummary, cfg routing_strategy_setting.RoutingStrategySetting) []RoutingStrategyScore {
	excluded := routingStrategyCSVSet(cfg.ExcludedGroups)
	pinned := routingStrategyCSVList(cfg.PinnedGroups)
	scores := make([]RoutingStrategyScore, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || excluded[group] {
			continue
		}
		perf := perfByGroup[group]
		groupRatio := ratio_setting.GetGroupRatio(group)
		score := RoutingStrategyScore{
			Group:        group,
			GroupRatio:   groupRatio,
			SuccessRate:  perf.SuccessRate,
			LatencyMs:    float64(perf.AvgLatencyMs),
			RequestCount: perf.RequestCount,
		}
		score.Score = calculateRoutingStrategyScore(strategy, score, cfg)
		scores = append(scores, score)
	}
	sort.SliceStable(scores, func(i, j int) bool {
		leftPinned := routingStrategyPinIndex(pinned, scores[i].Group)
		rightPinned := routingStrategyPinIndex(pinned, scores[j].Group)
		if leftPinned != rightPinned {
			if leftPinned < 0 {
				return false
			}
			if rightPinned < 0 {
				return true
			}
			return leftPinned < rightPinned
		}
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].Group < scores[j].Group
	})
	return scores
}

func calculateRoutingStrategyScore(strategy string, score RoutingStrategyScore, cfg routing_strategy_setting.RoutingStrategySetting) float64 {
	priceScore := 1 / math.Max(score.GroupRatio, 0.000001)
	successScore := score.SuccessRate / 100
	speedScore := 0.0
	if score.LatencyMs > 0 {
		speedScore = 1 / score.LatencyMs
	}
	if cfg.MinRequestCount > 0 && score.RequestCount > 0 && score.RequestCount < cfg.MinRequestCount {
		successScore *= float64(score.RequestCount) / float64(cfg.MinRequestCount)
		speedScore *= float64(score.RequestCount) / float64(cfg.MinRequestCount)
	}

	switch strategy {
	case model.RoutingStrategyPriceFirst:
		return priceScore
	case model.RoutingStrategySpeedFirst:
		return speedScore
	case model.RoutingStrategySuccessFirst:
		return successScore
	default:
		return priceScore*cfg.SmartPriceWeight + speedScore*cfg.SmartSpeedWeight + successScore*cfg.SmartSuccessWeight
	}
}

func routingStrategyUserGroups() []string {
	names, configured := setting.GetUserGroupNamesFromDisplayConfig()
	if configured && len(names) > 0 {
		return names
	}
	groups := setting.GetUserUsableGroupsCopy()
	return routingStrategySortedGroupNames(groups)
}

func routingStrategySortedGroupNames(groups map[string]string) []string {
	result := make([]string, 0, len(groups))
	for group := range groups {
		if strings.TrimSpace(group) != "" {
			result = append(result, group)
		}
	}
	sort.Strings(result)
	return result
}

func routingStrategyCSVSet(raw string) map[string]bool {
	result := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result[item] = true
		}
	}
	return result
}

func routingStrategyCSVList(raw string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func routingStrategyPinIndex(pinned []string, group string) int {
	for i, item := range pinned {
		if item == group {
			return i
		}
	}
	return -1
}
