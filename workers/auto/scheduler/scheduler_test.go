package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPauseResume(t *testing.T) {
	var runs atomic.Int32
	s := New(10*time.Millisecond, func(context.Context) { runs.Add(1) })
	s.Start(0)
	defer s.Stop()

	waitFor(t, "first run", func() bool { return runs.Load() > 0 })

	s.Pause()
	if !s.IsPaused() {
		t.Fatal("IsPaused() = false right after Pause")
	}
	time.Sleep(30 * time.Millisecond) // let an in-flight run finish
	before := runs.Load()
	time.Sleep(60 * time.Millisecond)
	if after := runs.Load(); after != before {
		t.Fatalf("job ran %d times while paused", after-before)
	}

	// The last call wins, whatever order they come in.
	s.Resume()
	s.Pause()
	if !s.IsPaused() {
		t.Fatal("Resume then Pause left the scheduler running")
	}
	s.Pause()
	s.Resume()
	if s.IsPaused() {
		t.Fatal("Pause then Resume left the scheduler paused")
	}
	waitFor(t, "a run after Resume", func() bool { return runs.Load() > before })
}

func TestStopFromJob(t *testing.T) {
	var s *Scheduler
	stopped := make(chan struct{})
	s = New(10*time.Millisecond, func(context.Context) {
		// Stop waits for the loop that runs this job, so it has to be async.
		go func() {
			s.Stop()
			close(stopped)
		}()
	})
	s.Start(0)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop called from a job never returned")
	}
}
