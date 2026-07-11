//go:build windows

package main

import "golang.org/x/sys/windows"

func replaceFileAtomically(tempPath, finalPath string) error {
	temp, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	final, err := windows.UTF16PtrFromString(finalPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(temp, final, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
