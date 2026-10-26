// Package inflight combines concurrent identical work without retaining results.
package inflight

import (
	"context"
	"fmt"
	"sync"
)

type call[T any] struct {
	done     chan struct{}
	cancel   context.CancelFunc
	waiters  int
	finished bool
	value    T
	err      error
}

type Group[T any] struct {
	mu    sync.Mutex
	calls map[string]*call[T]
}

// Do cancels work only when its last waiter leaves. A canceled first caller
// cannot abort other callers, and a completed result is never cached.
func (g *Group[T]) Do(ctx context.Context, key string, work func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[string]*call[T])
	}
	c := g.calls[key]
	if c == nil {
		workCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		c = &call[T]{done: make(chan struct{}), cancel: cancel}
		g.calls[key] = c
		go func() {
			defer func() {
				if p := recover(); p != nil {
					c.err = fmt.Errorf("shared operation panicked: %v", p)
				}
				g.mu.Lock()
				c.finished = true
				if g.calls[key] == c {
					delete(g.calls, key)
				}
				close(c.done)
				g.mu.Unlock()
				cancel()
			}()
			c.value, c.err = work(workCtx)
		}()
	}
	c.waiters++
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		c.waiters--
		if c.waiters == 0 && !c.finished {
			if g.calls[key] == c {
				delete(g.calls, key)
			}
			c.cancel()
		}
	}()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-c.done:
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return c.value, c.err
	}
}
