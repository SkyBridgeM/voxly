package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomically writes content to a final path via a temporary file
// in the same directory, then atomically renames it. This ensures that the
// final path always contains a complete, valid file.
func WriteFileAtomically(finalPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(finalPath)
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(perm); err != nil {
		temp.Close()
		return fmt.Errorf("setting temp file permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := replaceFileAtomically(tempPath, finalPath); err != nil {
		return fmt.Errorf("replacing file atomically: %w", err)
	}
	return nil
}
