package state

import (
	"sync/atomic"
)

// PauseType identifies the kind of pause
type PauseType int

const (
	DefaultPause PauseType = iota
	DLPause
	RSSPause
)

// PauseState tracks multiple independent pause flags
type PauseState struct {
	defC atomic.Int64
	dlC  atomic.Int64
	rssC atomic.Int64
}

func (ps *PauseState) Set(pt PauseType, on bool) {
	var addr *atomic.Int64
	switch pt {
	case DefaultPause:
		addr = &ps.defC
	case DLPause:
		addr = &ps.dlC
	case RSSPause:
		addr = &ps.rssC
	default:
		return
	}

	if on {
		addr.Add(1)
	} else {
		// Use a loop to ensure we never decrement below zero
		for {
			current := addr.Load()
			if current <= 0 {
				return
			}
			if addr.CompareAndSwap(current, current-1) {
				return
			}
		}
	}
}

func (ps *PauseState) Get(pt PauseType) bool {
	switch pt {
	case DefaultPause:
		return ps.defC.Load() > 0
	case DLPause:
		return ps.dlC.Load() > 0
	case RSSPause:
		return ps.rssC.Load() > 0
	}
	return false
}

func (ps *PauseState) Any() bool {
	return ps.defC.Load() > 0 || ps.dlC.Load() > 0 || ps.rssC.Load() > 0
}
