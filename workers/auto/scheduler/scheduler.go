package scheduler

import (
	"context"
	"sync"
	"time"
)

// JobFn is the function that will be executed each tick. It receives a cancellable context.
type JobFn func(ctx context.Context)

// Scheduler is a reusable scheduler with pause/resume/stop support.
type Scheduler struct {
	interval    time.Duration
	job         JobFn
	ticker      *time.Ticker
	stopCh      chan struct{}
	paused      bool
	pausedMu    sync.RWMutex
	started     bool
	startStopMu sync.Mutex
	wg          sync.WaitGroup
	stopOnce    sync.Once

	// Per-job timeout when calling job(ctx). If zero, no timeout is applied.
	JobTimeout time.Duration
}

// New creates a new Scheduler.
// interval: tick interval (time between runs).
// job: function to execute on each tick. Should handle context cancellation.
func New(interval time.Duration, job JobFn) *Scheduler {
	return &Scheduler{
		interval:   interval,
		job:        job,
		stopCh:     make(chan struct{}),
		JobTimeout: 30 * time.Second, // default job timeout
	}
}

// Start runs the scheduler loop. initialDelay is applied once before the first tick.
// Calling Start when already started is a no-op.
func (s *Scheduler) Start(initialDelay time.Duration) {
	s.startStopMu.Lock()
	if s.started {
		s.startStopMu.Unlock()
		return
	}
	s.started = true
	// create a fresh stop channel so Stop can be called safely later
	s.stopCh = make(chan struct{})
	s.startStopMu.Unlock()

	s.ticker = time.NewTicker(s.interval)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// initial delay (can be zero)
		if initialDelay > 0 {
			select {
			case <-time.After(initialDelay):
			case <-s.stopCh:
				return
			}
		}

		for {
			select {
			case <-s.ticker.C:
				if s.IsPaused() {
					continue
				}
				// create job context; respect JobTimeout if set
				var ctx context.Context
				var cancel context.CancelFunc
				if s.JobTimeout > 0 {
					ctx, cancel = context.WithTimeout(context.Background(), s.JobTimeout)
				} else {
					ctx, cancel = context.WithCancel(context.Background())
				}
				// run job in same goroutine so we control concurrency (if you prefer concurrent runs,
				// wrap this in a goroutine and consider limiting concurrency with a semaphore)
				s.job(ctx)
				cancel()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop stops the scheduler and waits for the background goroutine(s) to exit.
// Stop is idempotent.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		// close/cleanup
		close(s.stopCh)
		if s.ticker != nil {
			s.ticker.Stop()
		}
		// Wait for goroutine(s) to finish
		s.wg.Wait()

		// mark as not started so Start could be called again if desired
		s.startStopMu.Lock()
		s.started = false
		s.startStopMu.Unlock()
	})
}

// Pause pauses the scheduler; the job is skipped on ticks until Resume.
// It sets the state directly so the last of Pause/Resume always wins.
func (s *Scheduler) Pause() {
	s.setPaused(true)
}

// Resume resumes the scheduler.
func (s *Scheduler) Resume() {
	s.setPaused(false)
}

// IsPaused returns true if the scheduler is currently paused.
func (s *Scheduler) IsPaused() bool {
	s.pausedMu.RLock()
	defer s.pausedMu.RUnlock()
	return s.paused
}

// setPaused is internal guarded setter.
func (s *Scheduler) setPaused(p bool) {
	s.pausedMu.Lock()
	s.paused = p
	s.pausedMu.Unlock()
}
