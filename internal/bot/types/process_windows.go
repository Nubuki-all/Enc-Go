package types

import (
	"os/exec"
	"strconv"
)

// killTree kills pid and its child processes.
func killTree(pid int) error {
	return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}
