package perfmetrics

import (
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestRelaySampleSeparatesAttemptLatencyFromRequestLatency(t *testing.T) {
	now := time.Now()
	info := &relaycommon.RelayInfo{OriginModelName: "model", UsingGroup: "backup", IsStream: true, StartTime: now.Add(-20 * time.Second), AttemptStartTime: now.Add(-200 * time.Millisecond), FirstResponseTime: now.Add(-100 * time.Millisecond)}
	sample := relaySample(info, true, 10, now)
	require.Equal(t, int64(200), sample.LatencyMs)
	require.Equal(t, int64(100), sample.TtftMs)
	require.Equal(t, "backup", sample.Group)
	require.True(t, sample.Success)
	info.BeginAttempt()
	require.False(t, info.HasSendResponse(), "prior attempts must not create a false first-token sample")
	failed := relaySample(info, false, 0, time.Now())
	require.False(t, failed.HasTtft)
	require.False(t, failed.Success)
}
