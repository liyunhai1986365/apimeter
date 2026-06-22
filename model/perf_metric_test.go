package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPerfMetricsGroupSummaryUsesQuotedGroupColumn(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Create(&PerfMetric{
		ModelName:      "gpt-4o",
		Group:          "official",
		BucketTs:       100,
		RequestCount:   2,
		SuccessCount:   1,
		TotalLatencyMs: 600,
		TtftSumMs:      120,
		TtftCount:      2,
		OutputTokens:   240,
		GenerationMs:   3000,
	}).Error)
	require.NoError(t, DB.Create(&PerfMetric{
		ModelName:      "claude-3-5-sonnet",
		Group:          "official",
		BucketTs:       100,
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 300,
		TtftSumMs:      80,
		TtftCount:      1,
		OutputTokens:   80,
		GenerationMs:   2000,
	}).Error)

	rows, err := GetPerfMetricsGroupSummary(0, 200, []string{"official"})

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "official", rows[0].Group)
	require.EqualValues(t, 3, rows[0].RequestCount)
	require.EqualValues(t, 2, rows[0].SuccessCount)
	require.EqualValues(t, 900, rows[0].TotalLatencyMs)
	require.EqualValues(t, 200, rows[0].TtftSumMs)
	require.EqualValues(t, 3, rows[0].TtftCount)
	require.EqualValues(t, 320, rows[0].OutputTokens)
	require.EqualValues(t, 5000, rows[0].GenerationMs)
}
