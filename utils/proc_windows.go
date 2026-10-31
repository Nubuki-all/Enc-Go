package utils

import (
	"os"
	"os/exec"
	"syscall"
)

// ReX restarts the bot. Windows has no exec, so it starts a new process with
// the same console and exits.
func ReX(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

// detachedProcAttr starts a child without a console in its own process group.
func detachedProcAttr() *syscall.SysProcAttr {
	const detachedProcess = 0x00000008 // DETACHED_PROCESS
	return &syscall.SysProcAttr{
		CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// SetProcessGroup is a no-op on Windows; Process.Kill uses taskkill /T to
// kill the whole tree.
func SetProcessGroup(cmd *exec.Cmd) {}
