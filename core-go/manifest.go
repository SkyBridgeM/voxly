package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// SaveManifest writes manifest.json to a task directory.
func SaveManifest(taskDir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	path := ManifestPath(taskDir)
	return os.WriteFile(path, data, 0644)
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
		// If manifest doesn't exist, create a minimal one.
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
