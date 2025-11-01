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
	Default atomic.Bool
	DL      atomic.Bool
	RSS     atomic.Bool
	defC    atomic.Int64
	dlC     atomic.Int64
	rssC    atomic.Int64
}

// Set turns the given pause type on or off
func (ps *PauseState) Set(pt PauseType, on bool) {
	switch pt {
	case DefaultPause:
		if on {
			ps.defC.Add(1)
		} else if l := ps.defC.Load(); l != 0 {
			ps.defC.Add(-1)
			if l > 1 {
				return
			}
		}
		ps.Default.Store(on)
	case DLPause:
		if on {
			ps.dlC.Add(1)
		} else if l := ps.dlC.Load(); l != 0 {
			ps.dlC.Add(-1)
			if l > 1 {
				return
			}
		}
		ps.DL.Store(on)
	case RSSPause:
		if on {
			ps.rssC.Add(1)
		} else if l := ps.rssC.Load(); l != 0 {
			ps.rssC.Add(-1)
			if l > 1 {
				return
			}
		}
		ps.RSS.Store(on)
	}
}

// Get returns whether the given pause type is on
func (ps *PauseState) Get(pt PauseType) bool {
	switch pt {
	case DefaultPause:
		return ps.Default.Load()
	case DLPause:
		return ps.DL.Load()
	case RSSPause:
		return ps.RSS.Load()
	}
	return false
}

// Any returns true if *any* pause flag is set
func (ps *PauseState) Any() bool {
	return ps.Default.Load() || ps.DL.Load() || ps.RSS.Load()
}
