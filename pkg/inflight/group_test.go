package inflight

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGroupCancellationAndNoRetainedResult(t *testing.T) {
	var g Group[int]
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	work := func(ctx context.Context) (int, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return 7, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := g.Do(ctx, "same", work); first <- err }()
	<-started
	second := make(chan int, 1)
	go func() { v, _ := g.Do(context.Background(), "same", work); second <- v }()
	require.Eventually(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.calls["same"].waiters == 2 }, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-first, context.Canceled)
	close(release)
	require.Equal(t, 7, <-second)
	require.EqualValues(t, 1, calls.Load())
	v, err := g.Do(context.Background(), "same", func(context.Context) (int, error) { return 9, nil })
	require.NoError(t, err)
	require.Equal(t, 9, v, "completed work must not become a cache")
}

func TestGroupCancelsWorkAfterLastWaiterAndIsolatesKeys(t *testing.T) {
	var g Group[int]
	started, stopped := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := g.Do(ctx, "user-one", func(work context.Context) (int, error) {
			close(started)
			<-work.Done()
			close(stopped)
			return 0, work.Err()
		})
		done <- err
	}()
	<-started
	v, err := g.Do(context.Background(), "user-two", func(context.Context) (int, error) { return 2, nil })
	require.NoError(t, err)
	require.Equal(t, 2, v)
	cancel()
	require.True(t, errors.Is(<-done, context.Canceled))
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("abandoned work was not canceled")
	}
}
