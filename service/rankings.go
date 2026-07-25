package service

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	rankingCacheTTL         = 5 * time.Minute
	rankingLeaderboardLimit = 20
	rankingHistoryLimit     = 10
	rankingVendorLimit      = 5
	rankingMoverLimit       = 6
	rankingOthersLabel      = "Others"
	rankingUnknownVendor    = "Unknown"

	rankingsDataVisibilityMasked      = "masked"
	rankingsDataVisibilityExact       = "exact"
	rankingsDataVisibilityHiddenExact = "hidden_exact"
)

type RankingsResponse struct {
	DataVisibility     string             `json:"data_visibility"`
	ExactData          bool               `json:"exact_data"`
	DisplayMetric      string             `json:"display_metric"`
	Models             []RankedModel      `json:"models"`
	Vendors            []RankedVendor     `json:"vendors"`
	TopMovers          []RankingMover     `json:"top_movers"`
	TopDroppers        []RankingMover     `json:"top_droppers"`
	ModelsHistory      ModelHistorySeries `json:"models_history"`
	VendorShareHistory VendorShareSeries  `json:"vendor_share_history"`
}

type RankedModel struct {
	Rank         int     `json:"rank"`
	PreviousRank *int    `json:"previous_rank,omitempty"`
	ModelName    string  `json:"model_name"`
	Vendor       string  `json:"vendor"`
	VendorIcon   string  `json:"vendor_icon,omitempty"`
	Category     string  `json:"category"`
	TotalTokens  int64   `json:"total_tokens"`
	Share        float64 `json:"share"`
	GrowthPct    float64 `json:"growth_pct"`
}

type RankedVendor struct {
	Rank        int     `json:"rank"`
	Vendor      string  `json:"vendor"`
	VendorIcon  string  `json:"vendor_icon,omitempty"`
	TotalTokens int64   `json:"total_tokens"`
	Share       float64 `json:"share"`
	GrowthPct   float64 `json:"growth_pct"`
	ModelsCount int     `json:"models_count"`
	TopModel    string  `json:"top_model"`
}

type RankingMover struct {
	ModelName   string  `json:"model_name"`
	Vendor      string  `json:"vendor"`
	VendorIcon  string  `json:"vendor_icon,omitempty"`
	RankDelta   int     `json:"rank_delta"`
	CurrentRank int     `json:"current_rank"`
	GrowthPct   float64 `json:"growth_pct"`
}

type ModelHistoryPoint struct {
	Ts     string `json:"ts"`
	Label  string `json:"label"`
	Model  string `json:"model"`
	Vendor string `json:"vendor"`
	Tokens int64  `json:"tokens"`
}

type ModelHistoryModel struct {
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	Total  int64  `json:"total"`
}

type ModelHistorySeries struct {
	Points  []ModelHistoryPoint `json:"points"`
	Models  []ModelHistoryModel `json:"models"`
	Buckets int                 `json:"buckets"`
}

type VendorSharePoint struct {
	Ts     string  `json:"ts"`
	Label  string  `json:"label"`
	Vendor string  `json:"vendor"`
	Share  float64 `json:"share"`
	Tokens int64   `json:"tokens"`
}

type VendorShareVendor struct {
	Name  string  `json:"name"`
	Total int64   `json:"total"`
	Share float64 `json:"share"`
}

type VendorShareSeries struct {
	Points  []VendorSharePoint  `json:"points"`
	Vendors []VendorShareVendor `json:"vendors"`
	Buckets int                 `json:"buckets"`
}

type rankingPeriodConfig struct {
	id          string
	duration    time.Duration
	bucketSize  int64
	labelLayout string
	hasPrevious bool
}

type rankingCacheItem struct {
	expiresAt time.Time
	data      *RankingsResponse
}

type rankingModelMeta struct {
	vendor     string
	vendorIcon string
}

type vendorAggregate struct {
	name           string
	icon           string
	totalTokens    int64
	previousTokens int64
	models         map[string]struct{}
	topModel       string
	topModelTokens int64
}

var (
	rankingCacheMu sync.Mutex
	rankingCache   = map[string]rankingCacheItem{}
)

func GetRankingsSnapshot(period string) (*RankingsResponse, error) {
	return GetRankingsSnapshotForRole(period, 0)
}

func GetRankingsSnapshotForRole(period string, role int) (*RankingsResponse, error) {
	config, err := rankingConfig(period)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	rankingCacheMu.Lock()
	if item, ok := rankingCache[config.id]; ok && now.Before(item.expiresAt) {
		rankingCacheMu.Unlock()
		return prepareRankingsSnapshotForRole(item.data, role), nil
	}
	rankingCacheMu.Unlock()

	data, err := buildRankingsSnapshot(config, now)
	if err != nil {
		return nil, err
	}

	rankingCacheMu.Lock()
	rankingCache[config.id] = rankingCacheItem{
		expiresAt: now.Add(rankingCacheTTL),
		data:      data,
	}
	rankingCacheMu.Unlock()

	return prepareRankingsSnapshotForRole(data, role), nil
}

func rankingConfig(period string) (rankingPeriodConfig, error) {
	switch period {
	case "", "week":
		return rankingPeriodConfig{id: "week", duration: 7 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	case "today":
		return rankingPeriodConfig{id: "today", duration: 24 * time.Hour, bucketSize: 3600, labelLayout: "15:04", hasPrevious: true}, nil
	case "month":
		return rankingPeriodConfig{id: "month", duration: 30 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	case "year":
		return rankingPeriodConfig{id: "year", duration: 365 * 24 * time.Hour, bucketSize: 7 * 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	case "all":
		return rankingPeriodConfig{id: "all", bucketSize: 30 * 24 * 3600, labelLayout: "Jan 2006"}, nil
	default:
		return rankingPeriodConfig{}, fmt.Errorf("invalid ranking period: %s", period)
	}
}

func buildRankingsSnapshot(config rankingPeriodConfig, now time.Time) (*RankingsResponse, error) {
	startTime, endTime := rankingTimeRange(config, now)
	currentLogs, err := model.GetRankingConsumeLogRows(startTime, endTime)
	if err != nil {
		return nil, err
	}

	var previousLogs []model.RankingConsumeLogRow
	if config.hasPrevious {
		previousStart, previousEnd := previousRankingTimeRange(config, startTime)
		previousLogs, err = model.GetRankingConsumeLogRows(previousStart, previousEnd)
		if err != nil {
			return nil, err
		}
	}

	pricings := model.GetPricing()
	aliasMap := buildRankingAliasMap(pricings)
	modelPrimaryMap, err := model.GetModelPrimaryNameMap(collectRankingLogModelNames(currentLogs, previousLogs))
	if err != nil {
		return nil, err
	}
	for alias, primary := range modelPrimaryMap {
		if strings.TrimSpace(alias) == "" || strings.TrimSpace(primary) == "" {
			continue
		}
		aliasMap[alias] = primary
	}

	currentTotals, currentBuckets := buildRankingUsageFromLogs(currentLogs, config.bucketSize, aliasMap)
	previousTotals, _ := buildRankingUsageFromLogs(previousLogs, config.bucketSize, aliasMap)

	meta := buildRankingModelMeta(pricings)
	totalTokens := sumRankingTokens(currentTotals)
	previousRankByModel := rankingRankMap(previousTotals)
	previousTokensByModel := rankingTokenMap(previousTotals)

	rankedModels := buildRankedModels(currentTotals, totalTokens, previousRankByModel, previousTokensByModel, meta, config.hasPrevious)
	vendors := buildRankedVendors(currentTotals, previousTotals, totalTokens, meta, config.hasPrevious)
	modelHistory := buildModelHistory(currentBuckets, currentTotals, meta, config)
	vendorHistory := buildVendorShareHistory(currentBuckets, vendors, totalTokens, meta, config)
	movers, droppers := buildRankingMovers(rankedModels)

	return &RankingsResponse{
		DataVisibility:     rankingsDataVisibilityExact,
		ExactData:          true,
		DisplayMetric:      "tokens",
		Models:             limitRankedModels(rankedModels, rankingLeaderboardLimit),
		Vendors:            vendors,
		TopMovers:          movers,
		TopDroppers:        droppers,
		ModelsHistory:      modelHistory,
		VendorShareHistory: vendorHistory,
	}, nil
}

func prepareRankingsSnapshotForRole(data *RankingsResponse, role int) *RankingsResponse {
	if data == nil {
		return nil
	}
	visibility := getRankingsDataVisibility()
	if shouldUseExactRankingsData(visibility, role) {
		exact := cloneRankingsSnapshot(data)
		exact.DataVisibility = rankingsDataVisibilityExact
		exact.ExactData = true
		exact.DisplayMetric = "tokens"
		return exact
	}
	return maskRankingsSnapshot(data)
}

func getRankingsDataVisibility() string {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["RankingsDataVisibility"]
	common.OptionMapRWMutex.RUnlock()
	switch strings.TrimSpace(raw) {
	case rankingsDataVisibilityExact:
		return rankingsDataVisibilityExact
	case rankingsDataVisibilityHiddenExact:
		return rankingsDataVisibilityHiddenExact
	default:
		return rankingsDataVisibilityMasked
	}
}

func shouldUseExactRankingsData(visibility string, role int) bool {
	switch visibility {
	case rankingsDataVisibilityExact:
		return true
	case rankingsDataVisibilityHiddenExact:
		return role >= common.RoleAdminUser
	default:
		return false
	}
}

func maskRankingsSnapshot(data *RankingsResponse) *RankingsResponse {
	masked := cloneRankingsSnapshot(data)
	masked.DataVisibility = rankingsDataVisibilityMasked
	masked.ExactData = false
	masked.DisplayMetric = "popularity"

	modelMax := maxRankedModelTokens(masked.Models)
	for idx := range masked.Models {
		masked.Models[idx].TotalTokens = rankingPopularityScore(masked.Models[idx].TotalTokens, modelMax)
		masked.Models[idx].GrowthPct = rankingTrendScore(masked.Models[idx].GrowthPct)
	}
	recalculateRankedModelShares(masked.Models)

	vendorMax := maxRankedVendorTokens(masked.Vendors)
	for idx := range masked.Vendors {
		masked.Vendors[idx].TotalTokens = rankingPopularityScore(masked.Vendors[idx].TotalTokens, vendorMax)
		masked.Vendors[idx].GrowthPct = rankingTrendScore(masked.Vendors[idx].GrowthPct)
	}
	recalculateRankedVendorShares(masked.Vendors)

	for idx := range masked.TopMovers {
		masked.TopMovers[idx].GrowthPct = rankingTrendScore(masked.TopMovers[idx].GrowthPct)
	}
	for idx := range masked.TopDroppers {
		masked.TopDroppers[idx].GrowthPct = rankingTrendScore(masked.TopDroppers[idx].GrowthPct)
	}

	historyModelMax := maxModelHistoryTokens(masked.ModelsHistory)
	for idx := range masked.ModelsHistory.Points {
		masked.ModelsHistory.Points[idx].Tokens = rankingPopularityScore(masked.ModelsHistory.Points[idx].Tokens, historyModelMax)
	}
	for idx := range masked.ModelsHistory.Models {
		masked.ModelsHistory.Models[idx].Total = rankingPopularityScore(masked.ModelsHistory.Models[idx].Total, modelMax)
	}

	historyVendorMax := maxVendorShareHistoryTokens(masked.VendorShareHistory)
	for idx := range masked.VendorShareHistory.Points {
		masked.VendorShareHistory.Points[idx].Tokens = rankingPopularityScore(masked.VendorShareHistory.Points[idx].Tokens, historyVendorMax)
	}
	recalculateVendorShareHistory(masked.VendorShareHistory.Points)
	for idx := range masked.VendorShareHistory.Vendors {
		masked.VendorShareHistory.Vendors[idx].Total = rankingPopularityScore(masked.VendorShareHistory.Vendors[idx].Total, vendorMax)
	}
	recalculateVendorShareVendorShares(masked.VendorShareHistory.Vendors)

	return masked
}

func rankingTimeRange(config rankingPeriodConfig, now time.Time) (int64, int64) {
	endTime := now.Unix()
	if config.duration <= 0 {
		return 0, endTime
	}
	return now.Add(-config.duration).Unix(), endTime
}

func previousRankingTimeRange(config rankingPeriodConfig, currentStart int64) (int64, int64) {
	previousEnd := currentStart - 1
	previousStart := time.Unix(currentStart, 0).Add(-config.duration).Unix()
	return previousStart, previousEnd
}

func buildRankingModelMeta(pricings []model.Pricing) map[string]rankingModelMeta {
	vendorByID := make(map[int]model.PricingVendor)
	for _, vendor := range model.GetVendors() {
		vendorByID[vendor.ID] = vendor
	}

	meta := make(map[string]rankingModelMeta)
	for _, pricing := range pricings {
		item := rankingModelMeta{vendor: rankingUnknownVendor}
		if vendor, ok := vendorByID[pricing.VendorID]; ok {
			item.vendor = vendor.Name
			item.vendorIcon = vendor.Icon
		} else if pricing.OwnerBy != "" {
			item.vendor = pricing.OwnerBy
		}
		meta[pricing.ModelName] = item
	}
	return meta
}

func buildRankingAliasMap(pricings []model.Pricing) map[string]string {
	aliasToPrimary := make(map[string]string)
	for _, pricing := range pricings {
		primary := strings.TrimSpace(pricing.ModelName)
		if primary == "" {
			continue
		}
		if _, exists := aliasToPrimary[primary]; !exists {
			aliasToPrimary[primary] = primary
		}
		for _, alias := range pricing.AliasModels {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			if _, exists := aliasToPrimary[alias]; !exists {
				aliasToPrimary[alias] = primary
			}
		}
	}
	return aliasToPrimary
}

func canonicalRankingModelName(modelName string, aliasMap map[string]string) string {
	if primary, ok := aliasMap[modelName]; ok && primary != "" {
		return primary
	}
	return modelName
}

func collectRankingLogModelNames(groups ...[]model.RankingConsumeLogRow) []string {
	names := make([]string, 0)
	for _, rows := range groups {
		for _, row := range rows {
			if modelName := strings.TrimSpace(row.ModelName); modelName != "" {
				names = append(names, modelName)
			}
			if upstreamModelName := rankingLogUpstreamModelName(row.Other); upstreamModelName != "" {
				names = append(names, upstreamModelName)
			}
		}
	}
	return names
}

func buildRankingUsageFromLogs(rows []model.RankingConsumeLogRow, bucketSize int64, aliasMap map[string]string) ([]model.RankingQuotaTotal, []model.RankingQuotaBucket) {
	if len(rows) == 0 {
		return []model.RankingQuotaTotal{}, []model.RankingQuotaBucket{}
	}
	if bucketSize <= 0 {
		bucketSize = 3600
	}

	tokensByModel := make(map[string]int64)
	tokensByBucketAndModel := make(map[int64]map[string]int64)
	for _, row := range rows {
		inputTokens := row.InputTokens
		if inputTokens <= 0 {
			inputTokens = row.PromptTokens
		}
		tokens := int64(inputTokens) + int64(row.CompletionTokens)
		if tokens <= 0 {
			continue
		}
		modelName := rankingLogModelName(row, aliasMap)
		if modelName == "" {
			continue
		}
		tokensByModel[modelName] += tokens

		bucket := (row.CreatedAt / bucketSize) * bucketSize
		if _, ok := tokensByBucketAndModel[bucket]; !ok {
			tokensByBucketAndModel[bucket] = make(map[string]int64)
		}
		tokensByBucketAndModel[bucket][modelName] += tokens
	}

	totals := make([]model.RankingQuotaTotal, 0, len(tokensByModel))
	for modelName, totalTokens := range tokensByModel {
		if totalTokens <= 0 {
			continue
		}
		totals = append(totals, model.RankingQuotaTotal{
			ModelName:   modelName,
			TotalTokens: totalTokens,
		})
	}
	sort.Slice(totals, func(i, j int) bool {
		if totals[i].TotalTokens == totals[j].TotalTokens {
			return totals[i].ModelName < totals[j].ModelName
		}
		return totals[i].TotalTokens > totals[j].TotalTokens
	})

	buckets := make([]model.RankingQuotaBucket, 0, len(rows))
	for bucket, tokensByModel := range tokensByBucketAndModel {
		for modelName, tokens := range tokensByModel {
			if tokens <= 0 {
				continue
			}
			buckets = append(buckets, model.RankingQuotaBucket{
				ModelName: modelName,
				Bucket:    bucket,
				Tokens:    tokens,
			})
		}
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Bucket == buckets[j].Bucket {
			return buckets[i].ModelName < buckets[j].ModelName
		}
		return buckets[i].Bucket < buckets[j].Bucket
	})

	return totals, buckets
}

func rankingLogModelName(row model.RankingConsumeLogRow, aliasMap map[string]string) string {
	modelName := strings.TrimSpace(row.ModelName)
	upstreamModelName := rankingLogUpstreamModelName(row.Other)
	for _, candidate := range []string{upstreamModelName, modelName} {
		if primary, ok := aliasMap[candidate]; ok && strings.TrimSpace(primary) != "" {
			return primary
		}
	}
	if upstreamModelName != "" {
		return upstreamModelName
	}
	return modelName
}

func rankingLogUpstreamModelName(other string) string {
	other = strings.TrimSpace(other)
	if other == "" {
		return ""
	}
	otherMap, err := common.StrToMap(other)
	if err != nil || otherMap == nil {
		return ""
	}
	value, ok := otherMap["upstream_model_name"]
	if !ok {
		return ""
	}
	upstreamModelName, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(upstreamModelName)
}

func mergeRankingQuotaTotals(totals []model.RankingQuotaTotal, aliasMap map[string]string) []model.RankingQuotaTotal {
	if len(totals) == 0 {
		return totals
	}
	tokensByModel := make(map[string]int64, len(totals))
	for _, item := range totals {
		modelName := canonicalRankingModelName(item.ModelName, aliasMap)
		tokensByModel[modelName] += item.TotalTokens
	}

	rows := make([]model.RankingQuotaTotal, 0, len(tokensByModel))
	for modelName, totalTokens := range tokensByModel {
		if modelName == "" || totalTokens <= 0 {
			continue
		}
		rows = append(rows, model.RankingQuotaTotal{
			ModelName:   modelName,
			TotalTokens: totalTokens,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens == rows[j].TotalTokens {
			return rows[i].ModelName < rows[j].ModelName
		}
		return rows[i].TotalTokens > rows[j].TotalTokens
	})
	return rows
}

func mergeRankingQuotaBuckets(buckets []model.RankingQuotaBucket, aliasMap map[string]string) []model.RankingQuotaBucket {
	if len(buckets) == 0 {
		return buckets
	}
	tokensByBucketAndModel := make(map[int64]map[string]int64)
	for _, item := range buckets {
		modelName := canonicalRankingModelName(item.ModelName, aliasMap)
		if modelName == "" {
			continue
		}
		if _, ok := tokensByBucketAndModel[item.Bucket]; !ok {
			tokensByBucketAndModel[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndModel[item.Bucket][modelName] += item.Tokens
	}

	rows := make([]model.RankingQuotaBucket, 0, len(buckets))
	for bucket, tokensByModel := range tokensByBucketAndModel {
		for modelName, tokens := range tokensByModel {
			if tokens <= 0 {
				continue
			}
			rows = append(rows, model.RankingQuotaBucket{
				ModelName: modelName,
				Bucket:    bucket,
				Tokens:    tokens,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Bucket == rows[j].Bucket {
			return rows[i].ModelName < rows[j].ModelName
		}
		return rows[i].Bucket < rows[j].Bucket
	})
	return rows
}

func modelMeta(modelName string, meta map[string]rankingModelMeta) rankingModelMeta {
	if item, ok := meta[modelName]; ok && item.vendor != "" {
		return item
	}
	return rankingModelMeta{vendor: rankingUnknownVendor}
}

func buildRankedModels(totals []model.RankingQuotaTotal, totalTokens int64, previousRanks map[string]int, previousTokens map[string]int64, meta map[string]rankingModelMeta, showGrowth bool) []RankedModel {
	rows := make([]RankedModel, 0, len(totals))
	for idx, item := range totals {
		modelMeta := modelMeta(item.ModelName, meta)
		var previousRank *int
		if rank, ok := previousRanks[item.ModelName]; ok {
			rankCopy := rank
			previousRank = &rankCopy
		}
		growth := 0.0
		if showGrowth {
			growth = rankingGrowthPct(item.TotalTokens, previousTokens[item.ModelName])
		}
		rows = append(rows, RankedModel{
			Rank:         idx + 1,
			PreviousRank: previousRank,
			ModelName:    item.ModelName,
			Vendor:       modelMeta.vendor,
			VendorIcon:   modelMeta.vendorIcon,
			Category:     "all",
			TotalTokens:  item.TotalTokens,
			Share:        rankingShare(item.TotalTokens, totalTokens),
			GrowthPct:    growth,
		})
	}
	return rows
}

func buildRankedVendors(currentTotals []model.RankingQuotaTotal, previousTotals []model.RankingQuotaTotal, totalTokens int64, meta map[string]rankingModelMeta, showGrowth bool) []RankedVendor {
	aggregates := make(map[string]*vendorAggregate)
	for _, item := range currentTotals {
		modelMeta := modelMeta(item.ModelName, meta)
		agg := ensureVendorAggregate(aggregates, modelMeta)
		agg.totalTokens += item.TotalTokens
		agg.models[item.ModelName] = struct{}{}
		if item.TotalTokens > agg.topModelTokens {
			agg.topModel = item.ModelName
			agg.topModelTokens = item.TotalTokens
		}
	}
	for _, item := range previousTotals {
		modelMeta := modelMeta(item.ModelName, meta)
		agg := ensureVendorAggregate(aggregates, modelMeta)
		agg.previousTokens += item.TotalTokens
	}

	rows := make([]RankedVendor, 0, len(aggregates))
	for _, agg := range aggregates {
		if agg.totalTokens <= 0 {
			continue
		}
		growth := 0.0
		if showGrowth {
			growth = rankingGrowthPct(agg.totalTokens, agg.previousTokens)
		}
		rows = append(rows, RankedVendor{
			Vendor:      agg.name,
			VendorIcon:  agg.icon,
			TotalTokens: agg.totalTokens,
			Share:       rankingShare(agg.totalTokens, totalTokens),
			GrowthPct:   growth,
			ModelsCount: len(agg.models),
			TopModel:    agg.topModel,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens == rows[j].TotalTokens {
			return rows[i].Vendor < rows[j].Vendor
		}
		return rows[i].TotalTokens > rows[j].TotalTokens
	})
	for idx := range rows {
		rows[idx].Rank = idx + 1
	}
	return rows
}

func ensureVendorAggregate(aggregates map[string]*vendorAggregate, meta rankingModelMeta) *vendorAggregate {
	name := meta.vendor
	if name == "" {
		name = rankingUnknownVendor
	}
	agg, ok := aggregates[name]
	if !ok {
		agg = &vendorAggregate{
			name:   name,
			icon:   meta.vendorIcon,
			models: make(map[string]struct{}),
		}
		aggregates[name] = agg
	}
	if agg.icon == "" && meta.vendorIcon != "" {
		agg.icon = meta.vendorIcon
	}
	return agg
}

func buildModelHistory(buckets []model.RankingQuotaBucket, totals []model.RankingQuotaTotal, meta map[string]rankingModelMeta, config rankingPeriodConfig) ModelHistorySeries {
	topModels := make(map[string]struct{})
	models := make([]ModelHistoryModel, 0, minInt(len(totals), rankingHistoryLimit)+1)
	otherTotal := int64(0)
	for idx, item := range totals {
		if idx < rankingHistoryLimit {
			topModels[item.ModelName] = struct{}{}
			modelMeta := modelMeta(item.ModelName, meta)
			models = append(models, ModelHistoryModel{Name: item.ModelName, Vendor: modelMeta.vendor, Total: item.TotalTokens})
			continue
		}
		otherTotal += item.TotalTokens
	}
	if otherTotal > 0 {
		models = append(models, ModelHistoryModel{Name: rankingOthersLabel, Vendor: "Various", Total: otherTotal})
	}

	bucketSet := make(map[int64]struct{})
	tokensByBucketAndModel := make(map[int64]map[string]int64)
	for _, item := range buckets {
		modelName := item.ModelName
		if _, ok := topModels[modelName]; !ok {
			modelName = rankingOthersLabel
		}
		bucketSet[item.Bucket] = struct{}{}
		if _, ok := tokensByBucketAndModel[item.Bucket]; !ok {
			tokensByBucketAndModel[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndModel[item.Bucket][modelName] += item.Tokens
	}

	sortedBuckets := sortedRankingBuckets(bucketSet)
	points := make([]ModelHistoryPoint, 0, len(sortedBuckets)*len(models))
	for _, bucket := range sortedBuckets {
		for _, historyModel := range models {
			tokens := tokensByBucketAndModel[bucket][historyModel.Name]
			if tokens <= 0 {
				continue
			}
			points = append(points, ModelHistoryPoint{
				Ts:     rankingBucketTs(bucket),
				Label:  rankingBucketLabel(bucket, config),
				Model:  historyModel.Name,
				Vendor: historyModel.Vendor,
				Tokens: tokens,
			})
		}
	}

	return ModelHistorySeries{
		Points:  points,
		Models:  models,
		Buckets: len(sortedBuckets),
	}
}

func buildVendorShareHistory(buckets []model.RankingQuotaBucket, vendors []RankedVendor, totalTokens int64, meta map[string]rankingModelMeta, config rankingPeriodConfig) VendorShareSeries {
	topVendors := make(map[string]struct{})
	vendorRows := make([]VendorShareVendor, 0, minInt(len(vendors), rankingVendorLimit)+1)
	otherTotal := int64(0)
	for idx, vendor := range vendors {
		if idx < rankingVendorLimit {
			topVendors[vendor.Vendor] = struct{}{}
			vendorRows = append(vendorRows, VendorShareVendor{Name: vendor.Vendor, Total: vendor.TotalTokens, Share: vendor.Share})
			continue
		}
		otherTotal += vendor.TotalTokens
	}
	if otherTotal > 0 {
		vendorRows = append(vendorRows, VendorShareVendor{Name: rankingOthersLabel, Total: otherTotal, Share: rankingShare(otherTotal, totalTokens)})
	}

	bucketSet := make(map[int64]struct{})
	tokensByBucketAndVendor := make(map[int64]map[string]int64)
	totalsByBucket := make(map[int64]int64)
	for _, item := range buckets {
		modelMeta := modelMeta(item.ModelName, meta)
		vendorName := modelMeta.vendor
		if _, ok := topVendors[vendorName]; !ok {
			vendorName = rankingOthersLabel
		}
		bucketSet[item.Bucket] = struct{}{}
		if _, ok := tokensByBucketAndVendor[item.Bucket]; !ok {
			tokensByBucketAndVendor[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndVendor[item.Bucket][vendorName] += item.Tokens
		totalsByBucket[item.Bucket] += item.Tokens
	}

	sortedBuckets := sortedRankingBuckets(bucketSet)
	points := make([]VendorSharePoint, 0, len(sortedBuckets)*len(vendorRows))
	for _, bucket := range sortedBuckets {
		for _, vendor := range vendorRows {
			tokens := tokensByBucketAndVendor[bucket][vendor.Name]
			if tokens <= 0 {
				continue
			}
			points = append(points, VendorSharePoint{
				Ts:     rankingBucketTs(bucket),
				Label:  rankingBucketLabel(bucket, config),
				Vendor: vendor.Name,
				Share:  rankingShare(tokens, totalsByBucket[bucket]),
				Tokens: tokens,
			})
		}
	}

	return VendorShareSeries{
		Points:  points,
		Vendors: vendorRows,
		Buckets: len(sortedBuckets),
	}
}

func buildRankingMovers(models []RankedModel) ([]RankingMover, []RankingMover) {
	movers := make([]RankingMover, 0)
	droppers := make([]RankingMover, 0)
	for _, item := range models {
		if item.PreviousRank == nil {
			continue
		}
		delta := *item.PreviousRank - item.Rank
		if delta == 0 {
			continue
		}
		row := RankingMover{
			ModelName:   item.ModelName,
			Vendor:      item.Vendor,
			VendorIcon:  item.VendorIcon,
			RankDelta:   delta,
			CurrentRank: item.Rank,
			GrowthPct:   item.GrowthPct,
		}
		if delta > 0 {
			movers = append(movers, row)
		} else {
			droppers = append(droppers, row)
		}
	}
	sort.Slice(movers, func(i, j int) bool {
		if movers[i].RankDelta == movers[j].RankDelta {
			return movers[i].GrowthPct > movers[j].GrowthPct
		}
		return movers[i].RankDelta > movers[j].RankDelta
	})
	sort.Slice(droppers, func(i, j int) bool {
		if droppers[i].RankDelta == droppers[j].RankDelta {
			return droppers[i].GrowthPct < droppers[j].GrowthPct
		}
		return droppers[i].RankDelta < droppers[j].RankDelta
	})
	return limitRankingMovers(movers, rankingMoverLimit), limitRankingMovers(droppers, rankingMoverLimit)
}

func sortedRankingBuckets(bucketSet map[int64]struct{}) []int64 {
	buckets := make([]int64, 0, len(bucketSet))
	for bucket := range bucketSet {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i] < buckets[j]
	})
	return buckets
}

func rankingBucketTs(bucket int64) string {
	return time.Unix(bucket, 0).UTC().Format(time.RFC3339)
}

func rankingBucketLabel(bucket int64, config rankingPeriodConfig) string {
	return time.Unix(bucket, 0).Format(config.labelLayout)
}

func rankingRankMap(totals []model.RankingQuotaTotal) map[string]int {
	ranks := make(map[string]int, len(totals))
	for idx, item := range totals {
		ranks[item.ModelName] = idx + 1
	}
	return ranks
}

func rankingTokenMap(totals []model.RankingQuotaTotal) map[string]int64 {
	tokens := make(map[string]int64, len(totals))
	for _, item := range totals {
		tokens[item.ModelName] = item.TotalTokens
	}
	return tokens
}

func sumRankingTokens(totals []model.RankingQuotaTotal) int64 {
	total := int64(0)
	for _, item := range totals {
		total += item.TotalTokens
	}
	return total
}

func rankingShare(value int64, total int64) float64 {
	if total <= 0 || value <= 0 {
		return 0
	}
	return roundRankingFloat(float64(value) / float64(total))
}

func rankingGrowthPct(current int64, previous int64) float64 {
	if previous <= 0 {
		if current > 0 {
			return 100
		}
		return 0
	}
	return roundRankingFloat((float64(current-previous) / float64(previous)) * 100)
}

func roundRankingFloat(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func rankingPopularityScore(value int64, maxValue int64) int64 {
	if value <= 0 || maxValue <= 0 {
		return 0
	}
	score := int64(math.Round((float64(value) / float64(maxValue)) * 100))
	if score < 1 {
		return 1
	}
	if score > 100 {
		return 100
	}
	return score
}

func rankingTrendScore(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value == 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	if value < -100 {
		return -100
	}
	return roundRankingFloat(math.Round(value/10) * 10)
}

func maxRankedModelTokens(rows []RankedModel) int64 {
	var maxValue int64
	for _, row := range rows {
		if row.TotalTokens > maxValue {
			maxValue = row.TotalTokens
		}
	}
	return maxValue
}

func maxRankedVendorTokens(rows []RankedVendor) int64 {
	var maxValue int64
	for _, row := range rows {
		if row.TotalTokens > maxValue {
			maxValue = row.TotalTokens
		}
	}
	return maxValue
}

func maxModelHistoryTokens(series ModelHistorySeries) int64 {
	var maxValue int64
	for _, point := range series.Points {
		if point.Tokens > maxValue {
			maxValue = point.Tokens
		}
	}
	return maxValue
}

func maxVendorShareHistoryTokens(series VendorShareSeries) int64 {
	var maxValue int64
	for _, point := range series.Points {
		if point.Tokens > maxValue {
			maxValue = point.Tokens
		}
	}
	return maxValue
}

func recalculateRankedModelShares(rows []RankedModel) {
	total := int64(0)
	for _, row := range rows {
		total += row.TotalTokens
	}
	for idx := range rows {
		rows[idx].Share = rankingShare(rows[idx].TotalTokens, total)
	}
}

func recalculateRankedVendorShares(rows []RankedVendor) {
	total := int64(0)
	for _, row := range rows {
		total += row.TotalTokens
	}
	for idx := range rows {
		rows[idx].Share = rankingShare(rows[idx].TotalTokens, total)
	}
}

func recalculateVendorShareVendorShares(rows []VendorShareVendor) {
	total := int64(0)
	for _, row := range rows {
		total += row.Total
	}
	for idx := range rows {
		rows[idx].Share = rankingShare(rows[idx].Total, total)
	}
}

func recalculateVendorShareHistory(points []VendorSharePoint) {
	totalByTs := make(map[string]int64)
	for _, point := range points {
		totalByTs[point.Ts] += point.Tokens
	}
	for idx := range points {
		points[idx].Share = rankingShare(points[idx].Tokens, totalByTs[points[idx].Ts])
	}
}

func cloneRankingsSnapshot(data *RankingsResponse) *RankingsResponse {
	if data == nil {
		return nil
	}
	cloned := *data
	cloned.Models = append([]RankedModel(nil), data.Models...)
	cloned.Vendors = append([]RankedVendor(nil), data.Vendors...)
	cloned.TopMovers = append([]RankingMover(nil), data.TopMovers...)
	cloned.TopDroppers = append([]RankingMover(nil), data.TopDroppers...)
	cloned.ModelsHistory = ModelHistorySeries{
		Points:  append([]ModelHistoryPoint(nil), data.ModelsHistory.Points...),
		Models:  append([]ModelHistoryModel(nil), data.ModelsHistory.Models...),
		Buckets: data.ModelsHistory.Buckets,
	}
	cloned.VendorShareHistory = VendorShareSeries{
		Points:  append([]VendorSharePoint(nil), data.VendorShareHistory.Points...),
		Vendors: append([]VendorShareVendor(nil), data.VendorShareHistory.Vendors...),
		Buckets: data.VendorShareHistory.Buckets,
	}
	normalizeRankingsSnapshotSlices(&cloned)
	return &cloned
}

func normalizeRankingsSnapshotSlices(data *RankingsResponse) {
	if data.Models == nil {
		data.Models = []RankedModel{}
	}
	if data.Vendors == nil {
		data.Vendors = []RankedVendor{}
	}
	if data.TopMovers == nil {
		data.TopMovers = []RankingMover{}
	}
	if data.TopDroppers == nil {
		data.TopDroppers = []RankingMover{}
	}
	if data.ModelsHistory.Points == nil {
		data.ModelsHistory.Points = []ModelHistoryPoint{}
	}
	if data.ModelsHistory.Models == nil {
		data.ModelsHistory.Models = []ModelHistoryModel{}
	}
	if data.VendorShareHistory.Points == nil {
		data.VendorShareHistory.Points = []VendorSharePoint{}
	}
	if data.VendorShareHistory.Vendors == nil {
		data.VendorShareHistory.Vendors = []VendorShareVendor{}
	}
}

func limitRankedModels(rows []RankedModel, limit int) []RankedModel {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func limitRankingMovers(rows []RankingMover, limit int) []RankingMover {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
