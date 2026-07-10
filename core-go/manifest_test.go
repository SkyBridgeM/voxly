package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateManifest(t *testing.T) {
	taskDir := t.TempDir()
	sourceAudio := "/path/to/audio.wav"
	outputMode := "transcript"
	chunkMinutes := 5
	maxUploadMB := 10.0
	asrProvider := "mimo"
	asrModel := "mimo-v2.5-asr"

	manifest := CreateManifest(taskDir, sourceAudio, outputMode, chunkMinutes, maxUploadMB, asrProvider, asrModel)

	if manifest.Version != "1" {
		t.Errorf("expected version '1', got %q", manifest.Version)
	}
	if manifest.SourceAudio != sourceAudio {
		t.Errorf("expected source audio %q, got %q", sourceAudio, manifest.SourceAudio)
	}
	if manifest.OutputMode != outputMode {
		t.Errorf("expected output mode %q, got %q", outputMode, manifest.OutputMode)
	}
	if manifest.ChunkMinutes != chunkMinutes {
		t.Errorf("expected chunk minutes %d, got %d", chunkMinutes, manifest.ChunkMinutes)
	}
	if manifest.MaxUploadMB != maxUploadMB {
		t.Errorf("expected max upload MB %f, got %f", maxUploadMB, manifest.MaxUploadMB)
	}
	if manifest.ASRProvider != asrProvider {
		t.Errorf("expected ASR provider %q, got %q", asrProvider, manifest.ASRProvider)
	}
	if manifest.ASRModel != asrModel {
		t.Errorf("expected ASR model %q, got %q", asrModel, manifest.ASRModel)
	}
	if manifest.Status != "running" {
		t.Errorf("expected status 'running', got %q", manifest.Status)
	}
	if manifest.Error != nil {
		t.Errorf("expected nil error, got %v", manifest.Error)
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	taskDir := t.TempDir()

	manifest := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10.0, "mimo", "mimo-v2.5-asr")

	// Save manifest.
	if err := SaveManifest(taskDir, manifest); err != nil {
		t.Fatal(err)
	}

	// Load manifest.
	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Version != manifest.Version {
		t.Errorf("version mismatch: expected %q, got %q", manifest.Version, loaded.Version)
	}
	if loaded.SourceAudio != manifest.SourceAudio {
		t.Errorf("source audio mismatch: expected %q, got %q", manifest.SourceAudio, loaded.SourceAudio)
	}
	if loaded.Status != manifest.Status {
		t.Errorf("status mismatch: expected %q, got %q", manifest.Status, loaded.Status)
	}
}

func TestUpdateManifestStatus(t *testing.T) {
	taskDir := t.TempDir()

	manifest := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10.0, "mimo", "mimo-v2.5-asr")
	if err := SaveManifest(taskDir, manifest); err != nil {
		t.Fatal(err)
	}

	// Update status to completed.
	if err := UpdateManifestStatus(taskDir, "completed", nil); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", loaded.Status)
	}
	if loaded.Error != nil {
		t.Errorf("expected nil error, got %v", loaded.Error)
	}
}

func TestUpdateManifestStatus_Failed(t *testing.T) {
	taskDir := t.TempDir()

	manifest := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10.0, "mimo", "mimo-v2.5-asr")
	if err := SaveManifest(taskDir, manifest); err != nil {
		t.Fatal(err)
	}

	// Update status to failed with error.
	errMsg := "transcription failed"
	if err := UpdateManifestStatus(taskDir, "failed", &errMsg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", loaded.Status)
	}
	if loaded.Error == nil {
		t.Fatal("expected non-nil error")
	}
	if *loaded.Error != errMsg {
		t.Errorf("expected error %q, got %q", errMsg, *loaded.Error)
	}
}

func TestManifestPath(t *testing.T) {
	taskDir := "/path/to/task"
	expected := filepath.Join(taskDir, "manifest.json")
	result := ManifestPath(taskDir)

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestLoadManifest_NotFound(t *testing.T) {
	taskDir := t.TempDir()

	_, err := LoadManifest(taskDir)
	if err == nil {
		t.Fatal("expected error for non-existent manifest, got nil")
	}
}

func TestSaveManifestReplacesFileWithoutLeavingTemporaryFile(t *testing.T) {
	taskDir := t.TempDir()
	m := CreateManifest(taskDir, "/audio.wav", "transcript", 5, 10, "mimo", "model-a")
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}
	m.Status = "completed"
	if err := SaveManifest(taskDir, m); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "completed" {
		t.Fatalf("status = %q, want completed", loaded.Status)
	}
	temps, err := filepath.Glob(filepath.Join(taskDir, ".manifest-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary manifest files were not cleaned up: %v", temps)
	}
}

func TestUpdateManifestStatusDoesNotOverwriteCorruptManifest(t *testing.T) {
	taskDir := t.TempDir()
	path := ManifestPath(taskDir)
	original := "{not-json"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	err := UpdateManifestStatus(taskDir, "completed", nil)
	if err == nil || !strings.Contains(err.Error(), "parsing manifest") {
		t.Fatalf("expected parsing error, got %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != original {
		t.Fatalf("corrupt manifest was unexpectedly overwritten: %q", data)
	}
}

func TestPrepareManifestForResumeUpdatesEffectiveRun(t *testing.T) {
	taskDir := t.TempDir()
	existing := CreateManifest(taskDir, "/audio.wav", "minutes", 5, 10, "mimo", "old-model")
	existing.Status = "failed"
	errText := "old error"
	existing.Error = &errText

	if err := PrepareManifestForResume(taskDir, "transcript", "new-model", existing); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OutputMode != "transcript" || loaded.ASRModel != "new-model" || loaded.Status != "running" {
		t.Fatalf("unexpected resume manifest: %+v", loaded)
	}
	if loaded.Error != nil {
		t.Fatalf("resume manifest retained old error: %q", *loaded.Error)
	}
}
