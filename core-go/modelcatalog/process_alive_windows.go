//go:build windows

package modelcatalog

import "golang.org/x/sys/windows"

const processQueryLimitedInformation = 0x1000

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(handle)
	return true
}
