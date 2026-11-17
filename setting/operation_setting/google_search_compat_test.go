package operation_setting

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoogleSearchPriceCompatibility(t *testing.T) {
	old := toolPriceSetting.Prices
	t.Cleanup(func() { toolPriceSetting.Prices = old; RebuildToolPriceIndex() })
	for _, tc := range []struct {
		name, model     string
		prices          map[string]float64
		query, grounded float64
	}{
		{"default old", "gemini-2.5-pro", nil, 0, 35},
		{"default new", "gemini-3-flash", nil, 14, 0},
		{"global zero", "gemini-2.5-pro", map[string]float64{"google_search": 0}, 0, 0},
		{"scoped zero", "gemini-2.5-pro", map[string]float64{"google_search:gemini-2.5*": 0}, 0, 0},
		{"legacy request price", "gemini-2.5-pro", map[string]float64{"google_search": 10}, 0, 10},
		{"legacy new model price", "gemini-3-flash", map[string]float64{"google_search": 10}, 0, 10},
		{"longest legacy prefix", "gemini-2.5-pro", map[string]float64{"google_search": 10, "google_search:gemini*": 5, "google_search:gemini-2.5*": 2}, 0, 2},
		{"unmatched new rule", "gemini-2.5-pro", map[string]float64{"google_search": 10, "google_search_grounded_prompt:gemini-3*": 0}, 0, 10},
		{"explicit query pricing", "gemini-2.5-pro", map[string]float64{"google_search": 2, "google_search_grounded_prompt": 0}, 2, 0},
		{"explicit dual pricing", "gemini-2.5-pro", map[string]float64{"google_search": 2, "google_search_grounded_prompt": 3}, 2, 3},
		{"unmatched scoped price", "gemini-2.5-pro", map[string]float64{"google_search:gemini-3*": 0}, 0, 35},
		{"invalid legacy price", "gemini-2.5-pro", map[string]float64{"google_search": math.NaN()}, 0, 35},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toolPriceSetting.Prices = tc.prices
			RebuildToolPriceIndex()
			q, g := GetGoogleSearchPrices(tc.model)
			require.Equal(t, tc.query, q)
			require.Equal(t, tc.grounded, g)
			// Reloading the saved rules must retain their units and zero values.
			RebuildToolPriceIndex()
			q, g = GetGoogleSearchPrices(tc.model)
			require.Equal(t, tc.query, q)
			require.Equal(t, tc.grounded, g)
		})
	}
}
