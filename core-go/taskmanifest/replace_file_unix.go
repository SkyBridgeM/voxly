//go:build !windows

package taskmanifest

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
