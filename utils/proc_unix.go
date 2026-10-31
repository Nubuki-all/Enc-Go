//go:build !windows

package utils

import (
	"os"
	"os/exec"
	"syscall"
)

// ReX restarts the bot by replacing the current process.
func ReX(args ...string) error {
	cmd := append([]string{os.Args[0]}, args...)
	return syscall.Exec(os.Args[0], cmd, os.Environ())
}

// detachedProcAttr starts a child in a new session.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// SetProcessGroup puts cmd in its own process group so killing the group
// also kills what it starts (e.g. ffmpeg under sh -c).
func SetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
