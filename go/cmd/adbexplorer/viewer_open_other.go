//go:build !windows

package main

import "os/exec"

func launchFileOS(path string) error {
	return exec.Command("xdg-open", path).Start()
}
