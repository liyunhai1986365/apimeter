package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRankingsTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Vendor{}, &model.Channel{}, &model.Ability{}, &model.Log{}))

	originDB := model.DB
	originLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db
	model.InvalidatePricingCache()
	rankingCacheMu.Lock()
	rankingCache = map[string]rankingCacheItem{}
	rankingCacheMu.Unlock()
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		model.InvalidatePricingCache()
		rankingCacheMu.Lock()
		rankingCache = map[string]rankingCacheItem{}
		rankingCacheMu.Unlock()
	})
}

func TestBuildRankingsSnapshotAggregatesMappedLogsUnderModelPrimaryName(t *testing.T) {
	setupRankingsTestDB(t)
	now := time.Unix(1_800_000_000, 0)

	require.NoError(t, model.DB.Create(&model.Vendor{Id: 1, Name: "Main Vendor"}).Error)
	require.NoError(t, model.DB.Create(&model.Model{
		Id:          1,
		ModelName:   "gpt-main",
		VendorID:    1,
		AliasModels: "gpt-alias, upstream-gpt-main",
		Status:      1,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Name: "primary", Type: 1, Status: 1, Models: "gpt-main,gpt-alias", Group: "default"}).Error)
	require.NoError(t, model.DB.Create(&[]model.Ability{
		{Group: "default", Model: "gpt-main", ChannelId: 11, Enabled: true},
		{Group: "default", Model: "gpt-alias", ChannelId: 11, Enabled: true},
	}).Error)
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{
			CreatedAt:        now.Add(-time.Hour).Unix(),
			Type:             model.LogTypeConsume,
			ModelName:        "gpt-alias",
			PromptTokens:     10,
			CompletionTokens: 20,
			Other: common.MapToJsonStr(map[string]interface{}{
				"is_model_mapped":     true,
				"upstream_model_name": "upstream-gpt-main",
			}),
		},
		{
			CreatedAt:        now.Add(-30 * time.Minute).Unix(),
			Type:             model.LogTypeConsume,
			ModelName:        "gpt-alias",
			PromptTokens:     5,
			CompletionTokens: 7,
		},
	}).Error)

	snapshot, err := buildRankingsSnapshot(rankingPeriodConfig{
		id:          "week",
		duration:    7 * 24 * time.Hour,
		bucketSize:  24 * 3600,
		labelLayout: "Jan 2",
		hasPrevious: true,
	}, now)
	require.NoError(t, err)

	require.Len(t, snapshot.Models, 1)
	require.Equal(t, "gpt-main", snapshot.Models[0].ModelName)
	require.EqualValues(t, 42, snapshot.Models[0].TotalTokens)
	require.Len(t, snapshot.ModelsHistory.Models, 1)
	require.Equal(t, "gpt-main", snapshot.ModelsHistory.Models[0].Name)
	require.EqualValues(t, 42, snapshot.ModelsHistory.Models[0].Total)
	for _, point := range snapshot.ModelsHistory.Points {
		require.Equal(t, "gpt-main", point.Model)
	}
}

func TestMergeRankingQuotaTotalsAggregatesAliasesToPrimaryModel(t *testing.T) {
	aliasMap := buildRankingAliasMap([]model.Pricing{
		{ModelName: "gpt-main", AliasModels: []string{"gpt-main-v2", "gpt-main-preview"}},
		{ModelName: "other-model"},
	})

	totals := mergeRankingQuotaTotals([]model.RankingQuotaTotal{
		{ModelName: "gpt-main", TotalTokens: 100},
		{ModelName: "other-model", TotalTokens: 60},
		{ModelName: "gpt-main-v2", TotalTokens: 40},
		{ModelName: "gpt-main-preview", TotalTokens: 5},
	}, aliasMap)

	require.Equal(t, []model.RankingQuotaTotal{
		{ModelName: "gpt-main", TotalTokens: 145},
		{ModelName: "other-model", TotalTokens: 60},
	}, totals)
}

func TestMergeRankingQuotaTotalsSortsPreviousAliasTrafficUnderPrimaryModel(t *testing.T) {
	aliasMap := buildRankingAliasMap([]model.Pricing{
		{ModelName: "gpt-main", AliasModels: []string{"gpt-main-v2"}},
		{ModelName: "other-model"},
	})

	totals := mergeRankingQuotaTotals([]model.RankingQuotaTotal{
		{ModelName: "other-model", TotalTokens: 90},
		{ModelName: "gpt-main-v2", TotalTokens: 70},
		{ModelName: "gpt-main", TotalTokens: 25},
	}, aliasMap)

	require.Equal(t, []model.RankingQuotaTotal{
		{ModelName: "gpt-main", TotalTokens: 95},
		{ModelName: "other-model", TotalTokens: 90},
	}, totals)
}

func TestMergeRankingQuotaBucketsAggregatesAliasesPerBucket(t *testing.T) {
	aliasMap := buildRankingAliasMap([]model.Pricing{
		{ModelName: "gpt-main", AliasModels: []string{"gpt-main-v2"}},
		{ModelName: "other-model"},
	})

	buckets := mergeRankingQuotaBuckets([]model.RankingQuotaBucket{
		{ModelName: "gpt-main", Bucket: 1000, Tokens: 10},
		{ModelName: "gpt-main-v2", Bucket: 1000, Tokens: 15},
		{ModelName: "other-model", Bucket: 1000, Tokens: 7},
		{ModelName: "gpt-main-v2", Bucket: 2000, Tokens: 9},
	}, aliasMap)

	require.Equal(t, []model.RankingQuotaBucket{
		{ModelName: "gpt-main", Bucket: 1000, Tokens: 25},
		{ModelName: "other-model", Bucket: 1000, Tokens: 7},
		{ModelName: "gpt-main", Bucket: 2000, Tokens: 9},
	}, buckets)
}

func TestBuildRankedVendorsDoesNotCountAliasAsSeparateModelAfterMerge(t *testing.T) {
	aliasMap := buildRankingAliasMap([]model.Pricing{
		{ModelName: "gpt-main", AliasModels: []string{"gpt-main-v2"}},
		{ModelName: "other-model"},
	})
	currentTotals := mergeRankingQuotaTotals([]model.RankingQuotaTotal{
		{ModelName: "gpt-main", TotalTokens: 100},
		{ModelName: "gpt-main-v2", TotalTokens: 40},
		{ModelName: "other-model", TotalTokens: 60},
	}, aliasMap)
	previousTotals := mergeRankingQuotaTotals([]model.RankingQuotaTotal{
		{ModelName: "gpt-main-v2", TotalTokens: 70},
	}, aliasMap)

	vendors := buildRankedVendors(currentTotals, previousTotals, sumRankingTokens(currentTotals), map[string]rankingModelMeta{
		"gpt-main":    {vendor: "OpenAI"},
		"other-model": {vendor: "Other"},
	}, true)

	require.Len(t, vendors, 2)
	require.Equal(t, RankedVendor{
		Rank:        1,
		Vendor:      "OpenAI",
		TotalTokens: 140,
		Share:       0.7,
		GrowthPct:   100,
		ModelsCount: 1,
		TopModel:    "gpt-main",
	}, vendors[0])
}

func TestMaskRankingsSnapshotHidesExactTokenCounts(t *testing.T) {
	original := &RankingsResponse{
		Models: []RankedModel{
			{Rank: 1, ModelName: "gpt-main", TotalTokens: 9800, Share: 0.98, GrowthPct: 123.4},
			{Rank: 2, ModelName: "other-model", TotalTokens: 200, Share: 0.02, GrowthPct: -12.3},
		},
		Vendors: []RankedVendor{
			{Rank: 1, Vendor: "OpenAI", TotalTokens: 9800, Share: 0.98, GrowthPct: 123.4},
			{Rank: 2, Vendor: "Other", TotalTokens: 200, Share: 0.02, GrowthPct: -12.3},
		},
		TopMovers: []RankingMover{
			{ModelName: "gpt-main", GrowthPct: 123.4},
		},
		ModelsHistory: ModelHistorySeries{
			Points: []ModelHistoryPoint{
				{Ts: "2026-06-01T00:00:00Z", Label: "Jun 1", Model: "gpt-main", Tokens: 4900},
				{Ts: "2026-06-01T00:00:00Z", Label: "Jun 1", Model: "other-model", Tokens: 100},
			},
			Models: []ModelHistoryModel{
				{Name: "gpt-main", Total: 9800},
				{Name: "other-model", Total: 200},
			},
			Buckets: 1,
		},
		VendorShareHistory: VendorShareSeries{
			Points: []VendorSharePoint{
				{Ts: "2026-06-01T00:00:00Z", Label: "Jun 1", Vendor: "OpenAI", Share: 0.98, Tokens: 4900},
				{Ts: "2026-06-01T00:00:00Z", Label: "Jun 1", Vendor: "Other", Share: 0.02, Tokens: 100},
			},
			Vendors: []VendorShareVendor{
				{Name: "OpenAI", Total: 9800, Share: 0.98},
				{Name: "Other", Total: 200, Share: 0.02},
			},
			Buckets: 1,
		},
	}

	masked := maskRankingsSnapshot(original)

	require.False(t, masked.ExactData)
	require.Equal(t, rankingsDataVisibilityMasked, masked.DataVisibility)
	require.Equal(t, int64(100), masked.Models[0].TotalTokens)
	require.Equal(t, int64(2), masked.Models[1].TotalTokens)
	require.Equal(t, 0.9804, masked.Models[0].Share)
	require.Equal(t, 100.0, masked.Models[0].GrowthPct)
	require.Equal(t, -10.0, masked.Models[1].GrowthPct)
	require.Equal(t, int64(100), masked.ModelsHistory.Points[0].Tokens)
	require.Equal(t, int64(2), masked.ModelsHistory.Points[1].Tokens)
	require.Equal(t, int64(100), masked.Vendors[0].TotalTokens)
	require.Equal(t, int64(2), masked.Vendors[1].TotalTokens)
	require.Equal(t, int64(100), masked.VendorShareHistory.Points[0].Tokens)
	require.Equal(t, 0.9804, masked.VendorShareHistory.Points[0].Share)

	require.Equal(t, int64(9800), original.Models[0].TotalTokens)
	require.Equal(t, int64(4900), original.ModelsHistory.Points[0].Tokens)
}

func TestShouldUseExactRankingsData(t *testing.T) {
	require.True(t, shouldUseExactRankingsData(rankingsDataVisibilityExact, 0))
	require.False(t, shouldUseExactRankingsData(rankingsDataVisibilityMasked, 100))
	require.True(t, shouldUseExactRankingsData(rankingsDataVisibilityHiddenExact, 10))
	require.False(t, shouldUseExactRankingsData(rankingsDataVisibilityHiddenExact, 1))
}

func TestPrepareRankingsSnapshotReturnsEmptyArraysForNoData(t *testing.T) {
	prepared := prepareRankingsSnapshotForRole(&RankingsResponse{}, 0)

	require.NotNil(t, prepared.Models)
	require.NotNil(t, prepared.Vendors)
	require.NotNil(t, prepared.TopMovers)
	require.NotNil(t, prepared.TopDroppers)
	require.NotNil(t, prepared.ModelsHistory.Points)
	require.NotNil(t, prepared.ModelsHistory.Models)
	require.NotNil(t, prepared.VendorShareHistory.Points)
	require.NotNil(t, prepared.VendorShareHistory.Vendors)

	body, err := common.Marshal(prepared)
	require.NoError(t, err)
	require.Contains(t, string(body), `"models":[]`)
	require.Contains(t, string(body), `"vendors":[]`)
	require.Contains(t, string(body), `"points":[]`)
	require.NotContains(t, string(body), `null`)
}
