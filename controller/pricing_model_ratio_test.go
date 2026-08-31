package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestBuildPricingGroupModelRatiosReturnsOnlyOverrides(t *testing.T) {
	pricing := []model.Pricing{
		{ModelName: "glm-5.2", EnableGroup: []string{"alibaba"}},
		{ModelName: "qwen3.7", EnableGroup: []string{"all"}},
		{ModelName: "other", EnableGroup: []string{"alibaba"}},
	}
	base := map[string]float64{"alibaba": 1, "backup": 0.9}
	resolve := func(modelName, group string) float64 {
		if group == "alibaba" && modelName == "glm-5.2" {
			return 0.7
		}
		if group == "alibaba" && modelName == "qwen3.7" {
			return 0.5
		}
		return base[group]
	}

	got := buildPricingGroupModelRatios(pricing, base, resolve)
	require.Equal(t, map[string]map[string]float64{
		"alibaba": {
			"glm-5.2": 0.7,
			"qwen3.7": 0.5,
		},
	}, got)
}
