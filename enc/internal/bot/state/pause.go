package state

// PauseType identifies the kind of pause
type PauseType int

const (
	DefaultPause PauseType = iota
	DLPause
	RSSPause
)

// PauseState tracks multiple independent pause flags
type PauseState struct {
	Default bool
	DL      bool
	RSS     bool
}

// Set turns the given pause type on or off
func (ps *PauseState) Set(pt PauseType, on bool) {
	switch pt {
	case DefaultPause:
		ps.Default = on
	case DLPause:
		ps.DL = on
	case RSSPause:
		ps.RSS = on
	}
}

// Get returns whether the given pause type is on
func (ps *PauseState) Get(pt PauseType) bool {
	switch pt {
	case DefaultPause:
		return ps.Default
	case DLPause:
		return ps.DL
	case RSSPause:
		return ps.RSS
	}
	return false
}

// Any returns true if *any* pause flag is set
func (ps *PauseState) Any() bool {
	return ps.Default || ps.DL || ps.RSS
}
