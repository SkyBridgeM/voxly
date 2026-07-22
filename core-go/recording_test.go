package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordingLifecycleAndInterruptedScan(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	if err := runRecordingCommand("recording-init", []string{"--output-root", root}, &output); err != nil {
		t.Fatal(err)
	}
	var created recordingCommandResult
	if err := json.Unmarshal(output.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Status != "recording" || created.TaskFolder == "" || created.CheckpointsDir != filepath.Join(created.TaskFolder, "audio", "checkpoints") {
		t.Fatalf("created = %#v", created)
	}
	output.Reset()
	if err := runRecordingCommand("recording-update", []string{"--task-dir", created.TaskFolder, "--status", "paused", "--duration-ms", "1200", "--checkpoint-count", "1"}, &output); err != nil {
		t.Fatal(err)
	}
	var paused recordingCommandResult
	if err := json.Unmarshal(output.Bytes(), &paused); err != nil {
		t.Fatal(err)
	}
	if paused.Status != "paused" || paused.DurationMS != 1200 || paused.CheckpointCount != 1 {
		t.Fatalf("paused = %#v", paused)
	}
	output.Reset()
	if err := runRecordingCommand("recording-scan", []string{"--output-root", root}, &output); err != nil {
		t.Fatal(err)
	}
	var scanned []recordingCommandResult
	if err := json.Unmarshal(output.Bytes(), &scanned); err != nil {
		t.Fatal(err)
	}
	if len(scanned) != 1 || scanned[0].Status != "interrupted" {
		t.Fatalf("scanned = %#v", scanned)
	}
}

func TestRecordingScanDoesNotRewriteSettledManifest(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	if err := runRecordingCommand("recording-init", []string{"--output-root", root}, &output); err != nil {
		t.Fatal(err)
	}
	var created recordingCommandResult
	if err := json.Unmarshal(output.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := runRecordingCommand("recording-update", []string{"--task-dir", created.TaskFolder, "--status", "cancelled", "--duration-ms", "0", "--checkpoint-count", "0"}, &output); err != nil {
		t.Fatal(err)
	}
	manifestPath := ManifestPath(created.TaskFolder)
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	output.Reset()
	if err := runRecordingCommand("recording-scan", []string{"--output-root", root}, &output); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	infoAfter, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatal("recording scan rewrote a settled manifest")
	}
}

func TestRecordingUpdateRejectsFinalAudioThroughSymlink(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	if err := runRecordingCommand("recording-init", []string{"--output-root", root}, &output); err != nil {
		t.Fatal(err)
	}
	var created recordingCommandResult
	if err := json.Unmarshal(output.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideAudio := filepath.Join(outside, "recording.wav")
	if err := os.WriteFile(outsideAudio, []byte("not-a-real-wave"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(created.TaskFolder, "audio", "escaped.wav")
	if err := os.Symlink(outsideAudio, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	output.Reset()
	err := runRecordingCommand("recording-update", []string{
		"--task-dir", created.TaskFolder,
		"--status", "stopped",
		"--duration-ms", "1",
		"--checkpoint-count", "1",
		"--final-audio", link,
	}, &output)
	if err == nil {
		t.Fatal("expected symlinked final audio outside the task directory to be rejected")
	}
}
