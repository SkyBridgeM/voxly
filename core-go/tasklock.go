package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// TaskLock provides exclusive locking for a task directory to prevent
// concurrent resume operations from issuing duplicate API calls.
type TaskLock struct {
	path string
	file *os.File
}

// AcquireTaskLock creates an exclusive task-local lock file (.resume.lock).
// The lock contains the process ID and start time. If a stale lock is found
// (from a process that is no longer running), it is cleaned up automatically.
func AcquireTaskLock(taskDir string) (*TaskLock, error) {
	lockPath := filepath.Join(taskDir, ".resume.lock")

	// Try to create the lock file exclusively.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		// We created the lock. Write PID and timestamp.
		fmt.Fprintf(f, "%d\n%s\n", os.Getpid(), time.Now().Format(time.RFC3339))
		f.Close()
		return &TaskLock{path: lockPath}, nil
	}

	if !os.IsExist(err) {
		return nil, fmt.Errorf("creating lock file: %w", err)
	}

	// Lock file exists. Check if it's stale.
	data, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		return nil, fmt.Errorf("reading existing lock file: %w", readErr)
	}

	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) == 0 {
		// Empty lock file — treat as stale.
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("removing stale lock: %w", err)
		}
		return AcquireTaskLock(taskDir)
	}

	// Parse PID.
	var pid int
	if _, err := fmt.Sscanf(lines[0], "%d", &pid); err != nil {
		// Can't parse PID — treat as stale.
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("removing unparseable lock: %w", err)
		}
		return AcquireTaskLock(taskDir)
	}

	// Check if the process is still running.
	if isProcessAlive(pid) {
		return nil, fmt.Errorf("task is already being resumed (PID %d). Kill that process or remove %s", pid, lockPath)
	}

	// Stale lock — process is no longer running.
	fmt.Fprintf(os.Stderr, "Warning: removing stale lock from PID %d\n", pid)
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing stale lock: %w", err)
	}

	// Retry acquisition.
	return AcquireTaskLock(taskDir)
}

// Release removes the lock file.
func (l *TaskLock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.Remove(l.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("releasing lock: %w", err)
	}
	return nil
}

// isProcessAlive checks whether a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// ESRCH means no such process.
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return false
	}
	// EPERM means process exists but we don't have permission.
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}
