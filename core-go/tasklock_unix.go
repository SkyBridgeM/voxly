//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func lockTaskFile(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("task is already being resumed: %w", err)
	}
	return nil
}

func unlockTaskFile(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("unlocking task: %w", err)
	}
	return nil
}
