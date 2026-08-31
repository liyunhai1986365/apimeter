package controller

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/require"
)

func TestBuildHvoyProviderPricingResponseExpandsFinalGroupPrices(t *testing.T) {
	cacheRatio := 0.1
	createCacheRatio := 1.25
	updatedAt := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))

	response := buildHvoyProviderPricingResponse([]model.Pricing{
		{
			ModelName:        "claude-sonnet-4-6",
			QuotaType:        0,
			ModelRatio:       1.5,
			CompletionRatio:  5,
			CacheRatio:       &cacheRatio,
			CreateCacheRatio: &createCacheRatio,
			EnableGroup:      []string{"vip", "default"},
		},
		{
			ModelName:   "gpt-image-2",
			QuotaType:   1,
			ModelPrice:  0.2,
			EnableGroup: []string{"all"},
		},
	}, map[string]float64{"default": 1, "vip": 0.5}, func(group, modelName string) (float64, bool) {
		if group == "vip" && modelName == "claude-sonnet-4-6" {
			return 0.4, true
		}
		return 0, false
	}, 2, "Modelsell API", "api.example.com", updatedAt)

	require.True(t, response.Success)
	require.Equal(t, hvoyProviderPricingSchemaVersion, response.SchemaVersion)
	require.NotNil(t, response.Data)
	require.Equal(t, "CNY", response.Data.Currency)
	require.Equal(t, "2026-08-09T04:00:00Z", response.Data.UpdatedAt)
	require.Len(t, response.Data.Models, 4)

	claudeDefault := response.Data.Models[0]
	require.Equal(t, "default", claudeDefault.GroupName)
	require.Equal(t, 6.0, *claudeDefault.InputPrice)
	require.Equal(t, 30.0, *claudeDefault.OutputPrice)
	require.Equal(t, 0.6, *claudeDefault.CacheInputPrice)
	require.Equal(t, 7.5, *claudeDefault.CacheCreatePrice)
	require.Equal(t, 12.0, *claudeDefault.CacheCreatePrice1h)

	claudeVIP := response.Data.Models[1]
	require.Equal(t, "vip", claudeVIP.GroupName)
	require.Equal(t, 2.4, *claudeVIP.InputPrice)

	imageDefault := response.Data.Models[2]
	require.Equal(t, hvoyProviderPricingCallUnit, imageDefault.PriceUnit)
	require.Equal(t, 0.4, *imageDefault.UnitPrice)
	require.Nil(t, imageDefault.InputPrice)
	require.Equal(t, "vip", response.Data.Models[3].GroupName)
	require.Equal(t, 0.2, *response.Data.Models[3].UnitPrice)
}

func TestBuildHvoyProviderPricingResponseSkipsUnrepresentablePrices(t *testing.T) {
	response := buildHvoyProviderPricingResponse([]model.Pricing{
		{
			ModelName:   "dynamic-model",
			BillingMode: "tiered_expr",
			BillingExpr: `len < 1000 ? p * 1 : p * 2`,
			EnableGroup: []string{"default"},
		},
		{
			ModelName:   "free-per-call-model",
			QuotaType:   1,
			ModelPrice:  0,
			EnableGroup: []string{"default"},
		},
	}, map[string]float64{"default": 1}, nil, 1, "", "", time.Unix(0, 0))

	require.NotNil(t, response.Data)
	require.Empty(t, response.Data.Models)
}

func TestVerifyHvoyProviderPricingSignature(t *testing.T) {
	now := time.Unix(1_747_886_400, 0)
	secret := "test-secret"
	timestamp := "1747886400"
	validSignature := common.GenerateHMACWithKey([]byte(secret), timestamp)

	header := make(http.Header)
	header.Set("X-Hvoy-Ts", timestamp)
	header.Set("X-Hvoy-Sign", validSignature)
	require.NoError(t, verifyHvoyProviderPricingSignature(header, secret, now))
	require.NoError(t, verifyHvoyProviderPricingSignature(make(http.Header), "", now))

	header.Set("X-Hvoy-Sign", "invalid")
	require.EqualError(t, verifyHvoyProviderPricingSignature(header, secret, now), "invalid Hvoy signature")

	header.Set("X-Hvoy-Sign", validSignature)
	header.Set("X-Hvoy-Ts", "1747886339")
	require.EqualError(t, verifyHvoyProviderPricingSignature(header, secret, now), "expired Hvoy timestamp")
}
