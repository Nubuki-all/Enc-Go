package types

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
)

type Process struct {
	*os.Process
}

func NewProcess(p *os.Process) *Process {
	return &Process{p}
}

func (p *Process) Kill() error {
	if p.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(p.Pid)).Run()
	}
	err := syscall.Kill(-p.Pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return nil // Process already gone
	}
	return err
}
