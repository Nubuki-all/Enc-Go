package types

import (
	"context"
	"runtime"

	"golang.org/x/sync/semaphore"
)

// NoopMutex implements the sync.Locker interface but performs no actions.
type NoopMutex struct{}

func (m *NoopMutex) Lock()   {}
func (m *NoopMutex) Unlock() {}

// NoOpThrottler passes through immediately without blocking
type NoOpThrottler struct{}

func (n *NoOpThrottler) Acquire(ctx context.Context, weight int64) error { return nil }
func (n *NoOpThrottler) Release(weight int64)                            {}

type Throttler interface {
	Acquire(ctx context.Context, n int64) error
	Release(n int64)
}

func NewCPUThrottler(noOp bool, maxCap int64) Throttler {
	if noOp {
		return &NoOpThrottler{}
	}

	capacity := max(1, int64(runtime.NumCPU()/4))

	if maxCap > 0 {
		capacity = min(maxCap, capacity)
	}

	return semaphore.NewWeighted(capacity)
}
