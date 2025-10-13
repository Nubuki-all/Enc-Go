package utils

import (
	//"os"

	//"enc/config"
	"enc/internal/bot"
)

var (
	EInfo = NewEncodeInfo()
	EJob  = NewEncodeJob()
)

// DLInfo represents a download service job status
// Count: number of files
// FileList: list of file names
// Error: error message if any
// Name: torrent name
type DLInfo struct {
	Count int
	Files []string
	Error error
	Name  string
}

// String implements fmt.Stringer
func (d *DLInfo) String() string {
	if d.Error != nil {
		return d.Error.Error()
	}
	return d.Name
}

// EncodeInfo tracks encoding status
// Equivalent to Python's Encode_info
type EncodeInfo struct {
	Current  string
	Batch    bool
	CachedDL bool
	QBit     bool
	Select   any
	URI      any
	Previous string
}

// NewEncodeInfo creates and resets an EncodeInfo
func NewEncodeInfo() *EncodeInfo {
	ei := &EncodeInfo{}
	ei.Reset()
	return ei
}

// Reset clears the encoding info
func (e *EncodeInfo) Reset() {
	e.Previous = e.Current
	e.Current = ""
	e.Batch = false
	e.CachedDL = false
	e.QBit = false
	e.Select = nil
	e.URI = nil
}

// EncodeJob manages which ffmpeg scripts to run
// Equivalent to Python's Encode_job
type EncodeJob struct {
	ins          jobs
	Busy         bool
	ID           any
	SMInfo       any
	PrevDLClient any
}

// jobs holds flags for each ffmpeg script
type jobs struct {
	F1 bool
	F2 bool
	F3 bool
	F4 bool
}

// NewEncodeJob creates and resets an EncodeJob
func NewEncodeJob() *EncodeJob {
	ej := &EncodeJob{}
	ej.Reset(true)
	return ej
}

// Reset initializes the job flags; if !force and Busy, does nothing
func (e *EncodeJob) Reset(force bool) {
	if !force && e.Busy {
		return
	}
	e.ins = jobs{
		F1: true,
		F2: bot.FFmpeg2Args == "",
		F3: bot.FFmpeg3Args == "",
		F4: bot.FFmpeg4Args == "",
	}
	e.Busy = false
	e.ID = nil
	e.SMInfo = nil
	e.PrevDLClient = nil
}

// fileExists checks if the given path exists and is a file
//func fileExists(path string) bool {
//	info, err := os.Stat(path)
//	if err != nil {
//		return false
//	}
//	return !info.IsDir()
//}

// PendingInfo returns how many ffmpeg scripts are pending
type PendingInfo struct {
	Count int
	List  []string
}

// PendingInfo returns both count and list of pending ffmpeg files
func (e *EncodeJob) PendingInfo() PendingInfo {
	pending := []string{}
	if e.ins.F1 {
		pending = append(pending, bot.FFmpegArgs)
	}
	if e.ins.F2 {
		pending = append(pending, bot.FFmpeg2Args)
	}
	if e.ins.F3 {
		pending = append(pending, bot.FFmpeg3Args)
	}
	if e.ins.F4 {
		pending = append(pending, bot.FFmpeg4Args)
	}
	return PendingInfo{Count: len(pending), List: pending}
}

// Complete marks all pending jobs as done
func (e *EncodeJob) Complete() {
	for {
		pi := e.PendingInfo()
		if pi.Count == 0 {
			break
		}
		e.Done()
	}
}

// Done clears the next pending flag in order
func (e *EncodeJob) Done() {
	switch {
	case e.ins.F1:
		e.ins.F1 = false
	case e.ins.F2:
		e.ins.F2 = false
	case e.ins.F3:
		e.ins.F3 = false
	case e.ins.F4:
		e.ins.F4 = false
	}
}

// Pending returns the next ffmpeg file to run, empty if none
func (e *EncodeJob) Pending() string {
	switch {
	case e.ins.F1:
		return bot.FFmpegArgs
	case e.ins.F2:
		return bot.FFmpeg2Args
	case e.ins.F3:
		return bot.FFmpeg3Args
	case e.ins.F4:
		return bot.FFmpeg4Args
	default:
		return ""
	}
}

// PendingIndex returns the index (1-4) of next ffmpeg file, 0 if none
func (e *EncodeJob) PendingIndex() int {
	switch {
	case e.ins.F1:
		return 1
	case e.ins.F2:
		return 2
	case e.ins.F3:
		return 3
	case e.ins.F4:
		return 4
	default:
		return 0
	}
}

// PendingPos returns a textual position ("1st","2nd",etc.)
func (e *EncodeJob) PendingPos() string {
	switch e.PendingIndex() {
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	case 4:
		return "4th"
	default:
		return "1st"
	}
}
