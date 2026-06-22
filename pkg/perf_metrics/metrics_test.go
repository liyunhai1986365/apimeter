package perfmetrics

import "testing"

func TestBuildGroupSummaryResult(t *testing.T) {
	merged := map[bucketKey]counters{
		{model: "gpt-4o", group: "fast", bucketTs: 100}: {
			requestCount:   2,
			successCount:   1,
			totalLatencyMs: 600,
			ttftSumMs:      100,
			ttftCount:      1,
			outputTokens:   120,
			generationMs:   3000,
		},
		{model: "claude", group: "fast", bucketTs: 100}: {
			requestCount:   1,
			successCount:   1,
			totalLatencyMs: 300,
			ttftSumMs:      80,
			ttftCount:      1,
			outputTokens:   80,
			generationMs:   2000,
		},
		{model: "gpt-4o", group: "backup", bucketTs: 100}: {
			requestCount:   1,
			successCount:   0,
			totalLatencyMs: 1000,
		},
	}

	result := buildGroupSummaryResult(merged)

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}
	if result.Groups[0].Group != "fast" {
		t.Fatalf("expected fast to sort first by request count, got %s", result.Groups[0].Group)
	}
	if result.Groups[0].RequestCount != 3 {
		t.Fatalf("expected fast request count 3, got %d", result.Groups[0].RequestCount)
	}
	if result.Groups[0].AvgLatencyMs != 300 {
		t.Fatalf("expected fast avg latency 300, got %d", result.Groups[0].AvgLatencyMs)
	}
	if result.Groups[0].AvgTtftMs != 90 {
		t.Fatalf("expected fast avg ttft 90, got %d", result.Groups[0].AvgTtftMs)
	}
	if result.Groups[0].SuccessRate != 66.66666666666666 {
		t.Fatalf("expected fast success rate 66.666..., got %f", result.Groups[0].SuccessRate)
	}
	if result.Groups[0].AvgTps != 40 {
		t.Fatalf("expected fast avg tps 40, got %f", result.Groups[0].AvgTps)
	}
}
