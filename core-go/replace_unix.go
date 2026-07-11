//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func replaceFileAtomically(tempPath, finalPath string) error {
	if err := os.Rename(tempPath, finalPath); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(finalPath))
	if err != nil {
		return fmt.Errorf("opening parent directory for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("syncing parent directory: %w", err)
	}
	return nil
}
