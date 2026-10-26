package modelcatalog

import (
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TemporaryCleanup struct {
	RemovedBytes int64 `json:"removed_bytes"`
	RemovedCount int   `json:"removed_count"`
	SkippedCount int   `json:"skipped_count"`
}

func temporaryOwner(name string, staging bool, catalog Catalog) string {
	for _, model := range catalog.Models {
		prefix := model.ModelID + "."
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		token := strings.TrimPrefix(name, prefix)
		if staging {
			token = strings.TrimPrefix(token, "delete.")
		} else {
			if !strings.HasSuffix(token, ".partial") {
				continue
			}
			token = strings.TrimSuffix(token, ".partial")
		}
		if len(token) != 32 {
			continue
		}
		if _, err := hex.DecodeString(token); err == nil {
			return model.ModelID
		}
	}
	return ""
}

// Only catalog-owned, old download/staging entries are eligible. Taking the
// same cross-process lock as install/download prevents cleanup of live work.
func CleanupTemporary(root string, now time.Time, maxAge time.Duration) (TemporaryCleanup, error) {
	r := TemporaryCleanup{}
	if maxAge < 24*time.Hour {
		return r, errors.New("temporary retention must be at least 24 hours")
	}
	paths, err := NewPaths(root)
	if err != nil {
		return r, err
	}
	if _, err := os.Lstat(paths.Root); errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err := paths.prepare(); err != nil {
		return r, err
	}
	catalog, err := Load()
	if err != nil {
		return r, err
	}
	cutoff := now.Add(-maxAge)
	for _, dir := range []string{paths.Downloads, paths.Staging} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return r, err
		}
		for _, entry := range entries {
			owner := temporaryOwner(entry.Name(), dir == paths.Staging, catalog)
			if owner == "" {
				r.SkippedCount++
				continue
			}
			lock, err := acquireModelLock(paths, owner)
			if err != nil {
				r.SkippedCount++
				continue
			}
			path := filepath.Join(dir, entry.Name())
			var size int64
			eligible := true
			err = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
				if err != nil {
					eligible = false
					return err
				}
				info, err := d.Info()
				if err != nil {
					eligible = false
					return err
				}
				if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
					eligible = false
				}
				if info.Mode().IsRegular() {
					size += info.Size()
				}
				return nil
			})
			if err == nil && eligible {
				err = os.RemoveAll(path)
				if err == nil {
					r.RemovedCount++
					r.RemovedBytes += size
				} else {
					r.SkippedCount++
				}
			} else {
				r.SkippedCount++
			}
			lock.release()
		}
	}
	return r, nil
}
