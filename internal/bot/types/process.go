package types

import (
	"os"
)

type Process struct {
	*os.Process
}

func NewProcess(p *os.Process) *Process {
	return &Process{p}
}

// Kill kills the process and everything it started (ffmpeg under sh/cmd).
func (p *Process) Kill() error {
	if p.Process == nil {
		return nil
	}
	return killTree(p.Pid)
}
