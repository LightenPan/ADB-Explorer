//go:build windows

package main

import "golang.org/x/sys/windows"

func launchFileOS(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, p, nil, nil, windows.SW_SHOWNORMAL)
}
