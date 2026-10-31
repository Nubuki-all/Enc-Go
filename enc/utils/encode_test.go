package utils

import (
	"slices"
	"sync"
	"testing"
)

func TestEncodeInfoWorkers(t *testing.T) {
	e := NewEncodeInfo()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codec := []string{"x264", "x265"}[i%2]
			e.AddWorker(codec)
			_ = e.Codecs()
			e.AddFailed()
			e.RemoveWorker(codec)
		}()
	}
	wg.Wait()

	if got := e.Failed(); got != 50 {
		t.Errorf("Failed() = %d, want 50", got)
	}
	if got := e.Codecs(); len(got) != 0 {
		t.Errorf("Codecs() = %v, want none", got)
	}

	e.AddWorker("x265")
	e.AddWorker("x264")
	e.AddWorker("x265")
	e.RemoveWorker("x265") // removes one of the two
	if got := e.Codecs(); !slices.Equal(got, []string{"x264", "x265"}) {
		t.Errorf("Codecs() = %v, want [x264 x265]", got)
	}
	got := e.Codecs()
	got[0] = "changed"
	if e.Codecs()[0] != "x264" {
		t.Error("Codecs() returned the internal slice")
	}
}

func TestEncodeInfoResetKeepsEvent(t *testing.T) {
	e := NewEncodeInfo()
	ch := e.Event
	e.Event <- struct{}{}
	e.AddFailed()
	e.Reset()

	if e.Event != ch {
		t.Fatal("Reset replaced the Event channel")
	}
	select {
	case <-e.Event:
		t.Fatal("Reset left a stale event")
	default:
	}
	if e.Failed() != 0 || len(e.Codecs()) != 0 {
		t.Error("Reset did not clear the counters")
	}
}
