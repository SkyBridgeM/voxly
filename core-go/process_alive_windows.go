//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

const stillActive = 259

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	return windows.GetExitCodeProcess(handle, &exitCode) == nil && exitCode == stillActive
}
