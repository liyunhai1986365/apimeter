package controller

import (
	"context"
	"fmt"
	"time"
)

type readinessProbe struct {
	name  string
	check func(context.Context) error
}

type readinessResult struct {
	name     string
	duration time.Duration
	err      error
}

// Start every dependency before waiting, so a saturated main database pool
// cannot consume the log database's or Redis's entire readiness budget.
func runReadinessChecks(ctx context.Context, probes []readinessProbe) []readinessResult {
	started := time.Now()
	type completedProbe struct {
		index int
		readinessResult
	}
	completed := make(chan completedProbe, len(probes))
	for index, probe := range probes {
		go func() {
			start := time.Now()
			err := probe.check(ctx)
			// Buffered for every probe: a late client/driver return must not
			// block after the handler has exhausted its overall deadline.
			completed <- completedProbe{index, readinessResult{probe.name, time.Since(start), err}}
		}()
	}
	results := make([]readinessResult, len(probes))
	remaining := len(probes)
	record := func(result completedProbe) {
		results[result.index] = result.readinessResult
		remaining--
	}
	for remaining > 0 {
		select {
		case result := <-completed:
			record(result)
		case <-ctx.Done():
			// Preserve completed successes even when the deadline and a result
			// become readable together. Only unfinished checks are unavailable.
			for {
				select {
				case result := <-completed:
					record(result)
				default:
					for i, probe := range probes {
						if results[i].name == "" {
							results[i] = readinessResult{probe.name, time.Since(started), fmt.Errorf("readiness probe did not finish: %w", ctx.Err())}
						}
					}
					return results
				}
			}
		}
	}
	return results
}
