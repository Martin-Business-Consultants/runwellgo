//go:build windows

package main

import "os/exec"

// ownGroup: Windows has no process groups to signal; stopping the command
// kills it.
func ownGroup(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
