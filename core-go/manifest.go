package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Manifest represents the task manifest stored in each task directory.
type Manifest struct {
	Version      string  `json:"version"`
	SourceAudio  string  `json:"source_audio"`
	OutputMode   string  `json:"output_mode"`
	ChunkMinutes int     `json:"chunk_minutes"`
	MaxUploadMB  float64 `json:"max_upload_mb"`
	ASRProvider  string  `json:"asr_provider"`
	ASRModel     string  `json:"asr_model"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	Status       string  `json:"status"`
	Error        *string `json:"error"`
}

// ManifestPath returns the path to manifest.json in a task directory.
func ManifestPath(taskDir string) string {
	return filepath.Join(taskDir, "manifest.json")
}

// LoadManifest reads manifest.json from a task directory.
func LoadManifest(taskDir string) (*Manifest, error) {
	path := ManifestPath(taskDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return &m, nil
}

// SaveManifest atomically writes manifest.json to a task directory.
func SaveManifest(taskDir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	temp, err := os.CreateTemp(taskDir, ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary manifest: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0644); err != nil {
		temp.Close()
		return fmt.Errorf("setting manifest permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("writing temporary manifest: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("syncing temporary manifest: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing temporary manifest: %w", err)
	}
	destination := ManifestPath(taskDir)
	if err := os.Rename(tempPath, destination); err != nil {
		// Windows cannot rename over an existing file with os.Rename.
		if runtime.GOOS != "windows" {
			return fmt.Errorf("replacing manifest: %w", err)
		}
		if removeErr := os.Remove(destination); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("removing old manifest before replacement: %w", removeErr)
		}
		if renameErr := os.Rename(tempPath, destination); renameErr != nil {
			return fmt.Errorf("replacing manifest: %w", renameErr)
		}
	}
	return nil
}

// CreateManifest creates a new manifest for a task.
func CreateManifest(taskDir, sourceAudio, outputMode string, chunkMinutes int, maxUploadMB float64, asrProvider, asrModel string) *Manifest {
	now := time.Now().Format(time.RFC3339)
	return &Manifest{
		Version:      "1",
		SourceAudio:  sourceAudio,
		OutputMode:   outputMode,
		ChunkMinutes: chunkMinutes,
		MaxUploadMB:  maxUploadMB,
		ASRProvider:  asrProvider,
		ASRModel:     asrModel,
		CreatedAt:    now,
		UpdatedAt:    now,
		Status:       "running",
		Error:        nil,
	}
}

// UpdateManifestStatus updates the manifest status and updated_at timestamp.
func UpdateManifestStatus(taskDir, status string, errStr *string) error {
	m, loadErr := LoadManifest(taskDir)
	if loadErr != nil {
		if !errors.Is(loadErr, os.ErrNotExist) {
			return loadErr
		}
		// Legacy tasks may not have a manifest yet.
		m = &Manifest{
			Version:   "1",
			CreatedAt: time.Now().Format(time.RFC3339),
		}
	}

	m.Status = status
	m.UpdatedAt = time.Now().Format(time.RFC3339)
	m.Error = errStr

	return SaveManifest(taskDir, m)
}

// PrepareManifestForResume makes the manifest describe the effective resume run.
func PrepareManifestForResume(taskDir, outputMode, asrModel string, existing *Manifest) error {
	now := time.Now().Format(time.RFC3339)
	m := existing
	if m == nil {
		m = &Manifest{
			Version:     "1",
			ASRProvider: "mimo",
			CreatedAt:   now,
		}
	}
	m.OutputMode = outputMode
	m.ASRProvider = "mimo"
	m.ASRModel = asrModel
	m.Status = "running"
	m.Error = nil
	m.UpdatedAt = now
	return SaveManifest(taskDir, m)
}

func failTask(taskDir string, workflowErr error) error {
	if manifestErr := UpdateManifestStatus(taskDir, "failed", strPtr(workflowErr.Error())); manifestErr != nil {
		return fmt.Errorf("%w; additionally failed to update manifest: %v", workflowErr, manifestErr)
	}
	return workflowErr
}
