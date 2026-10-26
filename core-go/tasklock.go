package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TaskLock holds an OS-level exclusive lock for one task. The lock is released
// automatically when the process exits, so there is no stale-lock recovery
// race and no PID liveness heuristic.
type TaskLock struct {
	path string
	file *os.File
}

func AcquireTaskLock(taskDir string) (*TaskLock, error) {
	path := filepath.Join(taskDir, ".resume.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening task lock: %w", err)
	}
	if err := lockTaskFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Truncate(0); err != nil {
		_ = unlockTaskFile(f)
		_ = f.Close()
		return nil, fmt.Errorf("clearing task lock metadata: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		_ = unlockTaskFile(f)
		_ = f.Close()
		return nil, fmt.Errorf("seeking task lock metadata: %w", err)
	}
	if _, err := fmt.Fprintf(f, "%d\n%s\n", os.Getpid(), time.Now().Format(time.RFC3339)); err != nil {
		_ = unlockTaskFile(f)
		_ = f.Close()
		return nil, fmt.Errorf("writing task lock metadata: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = unlockTaskFile(f)
		_ = f.Close()
		return nil, fmt.Errorf("syncing task lock metadata: %w", err)
	}
	return &TaskLock{path: path, file: f}, nil
}

// ProbeTaskLock reports whether the Task lock is currently free, without
// creating or rewriting the lock file: Task Library discovery must stay
// read-only. It observes the same OS lock the runtime takes instead of guessing
// a process id, and it reports "free" whenever nothing observable holds the
// lock, because the runtime still refuses a real conflict when it locks.
func ProbeTaskLock(taskDir string) bool {
	path := filepath.Join(taskDir, ".resume.lock")
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return true
	}
	defer f.Close()
	if err := lockTaskFile(f); err != nil {
		return false
	}
	_ = unlockTaskFile(f)
	return true
}

func (l *TaskLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := unlockTaskFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil {
		return unlockErr
	}
	if closeErr != nil {
		return fmt.Errorf("closing task lock: %w", closeErr)
	}
	return nil
}
