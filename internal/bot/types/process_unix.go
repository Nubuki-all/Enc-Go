//go:build !windows

package types

import "syscall"

// killTree kills the process group led by pid (processes are started with
// Setpgid).
func killTree(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return nil // Process already gone
	}
	return err
}
