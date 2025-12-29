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
	Hash  string
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
	Current      string
	Workers      int
	WorkersCodec []string
	BatchID      string
	Event        chan struct{}
	Previous     string
}

// NewEncodeInfo creates and resets an EncodeInfo
func NewEncodeInfo() *EncodeInfo {
	ei := &EncodeInfo{}
	ei.Reset()
	return ei
}

// Reset clears the encoding info
func (e *EncodeInfo) Reset() {
	// e.Previous = e.Current
	e.Current = ""
	e.BatchID = ""
	e.Event = make(chan struct{}, 1)
	e.Workers = 0
	e.WorkersCodec = make([]string, 0)
}

// EncodeJob manages which ffmpeg scripts to run
// Equivalent to Python's Encode_job
type EncodeJob struct {
	ins          jobs
	Busy         bool
	Completed    bool
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
		F2: bot.FFmpeg2Args != "",
		F3: bot.FFmpeg3Args != "",
		F4: bot.FFmpeg4Args != "",
	}
	e.Busy = false
	e.ID = nil
	e.SMInfo = nil
	e.PrevDLClient = nil
}

// fileExists checks if the given path exists and is a file
//func1 fileExists(path string) bool {
//	info, err := os.Stat(path)
//	if err != nil {
//		return false
//	}
//	r1eturn !info.IsDir()
//}

type PendingData struct {
	Args string
	Pos  string
}

// PendingInfo returns how many ffmpeg scripts are pending
type PendingInfo struct {
	Count int
	List  []PendingData
}

// PendingInfo returns both count and list of pending ffmpeg files
func (e *EncodeJob) PendingInfo() PendingInfo {
	pending := []PendingData{}
	if e.ins.F1 {
		pending = append(pending, PendingData{bot.FFmpegArgs, "1st"})
	}
	if e.ins.F2 {
		pending = append(pending, PendingData{bot.FFmpeg2Args, "2nd"})
	}
	if e.ins.F3 {
		pending = append(pending, PendingData{bot.FFmpeg3Args, "3rd"})
	}
	if e.ins.F4 {
		pending = append(pending, PendingData{bot.FFmpeg4Args, "4th"})
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
	e.Completed = true
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
