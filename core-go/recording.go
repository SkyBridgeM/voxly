package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

type recordingCommandResult struct {
	TaskID          string `json:"task_id"`
	TaskFolder      string `json:"task_folder"`
	CheckpointsDir  string `json:"checkpoints_dir,omitempty"`
	AudioPath       string `json:"audio_path,omitempty"`
	Status          string `json:"status"`
	DurationMS      int64  `json:"duration_ms"`
	CheckpointCount int    `json:"checkpoint_count"`
}

func runRecordingCommand(command string, args []string, stdout io.Writer) error {
	switch command {
	case "recording-init":
		return runRecordingInit(args, stdout)
	case "recording-update":
		return runRecordingUpdate(args, stdout)
	case "recording-scan":
		return runRecordingScan(args, stdout)
	default:
		return fmt.Errorf("unknown recording command %q", command)
	}
}

func runRecordingInit(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("recording-init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("output-root", "", "Voxly output root")
	title := fs.String("title", "recording", "Safe task title seed")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || !filepath.IsAbs(*root) {
		return fmt.Errorf("--output-root must be absolute")
	}
	taskDir, err := CreateTaskDir(*root, *title)
	if err != nil {
		return err
	}
	checkpoints := filepath.Join(taskDir, "audio", "checkpoints")
	if err := os.Mkdir(checkpoints, 0700); err != nil {
		return fmt.Errorf("creating recording checkpoints: %w", err)
	}
	finalAudio := filepath.Join(taskDir, "audio", "recording.wav")
	m := CreateManifest(taskDir, finalAudio, "transcript_only", 5, 10, "unselected", "unselected")
	m.SourceKind = "microphone_recording"
	m.SourceOwnership = "captured"
	m.InputFormat = "wav"
	m.Source.OriginalName = "recording.wav"
	m.Recording = &taskmanifest.RecordingMetadata{
		Status: "recording", StartedAt: nowFunc().UTC().Format(time.RFC3339),
	}
	if err := SaveManifest(taskDir, m); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(recordingResult(taskDir, m))
}

func runRecordingUpdate(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("recording-update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	taskDir := fs.String("task-dir", "", "Recording task directory")
	status := fs.String("status", "", "recording status")
	duration := fs.Int64("duration-ms", 0, "captured duration")
	checkpointCount := fs.Int("checkpoint-count", 0, "durable checkpoint count")
	finalAudio := fs.String("final-audio", "", "Final WAV path")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || !filepath.IsAbs(*taskDir) {
		return fmt.Errorf("--task-dir must be absolute")
	}
	allowed := map[string]bool{"recording": true, "paused": true, "stopped": true, "interrupted": true, "cancelled": true, "recovered": true}
	if !allowed[*status] || *duration < 0 || *checkpointCount < 0 {
		return fmt.Errorf("invalid recording update")
	}
	m, err := LoadManifest(*taskDir)
	if err != nil {
		return err
	}
	if m.SourceKind != "microphone_recording" || m.Recording == nil {
		return fmt.Errorf("task is not a microphone recording")
	}
	m.Recording.Status, m.Recording.DurationMS, m.Recording.CheckpointCount = *status, *duration, *checkpointCount
	if *status == "stopped" || *status == "cancelled" || *status == "recovered" || *status == "interrupted" {
		m.Recording.StoppedAt = nowFunc().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(*finalAudio) != "" {
		canonical, err := filepath.EvalSymlinks(*finalAudio)
		if err != nil {
			return fmt.Errorf("final audio is unavailable")
		}
		audioDir, err := filepath.EvalSymlinks(filepath.Join(*taskDir, "audio"))
		if err != nil || filepath.Dir(canonical) != audioDir {
			return fmt.Errorf("final audio must be directly inside the task audio directory")
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("final audio is unavailable")
		}
		hash, err := fileSHA256(canonical)
		if err != nil {
			return err
		}
		m.Source.Path, m.Source.SizeBytes = canonical, info.Size()
		m.Source.OriginalName, m.InputFormat = filepath.Base(canonical), "wav"
		m.Recording.FinalAudioReference = filepath.ToSlash(filepath.Join("audio", filepath.Base(canonical)))
		m.Recording.FinalAudioFormat, m.Recording.FinalAudioSHA256 = "wav", hash
	}
	if err := SaveManifest(*taskDir, m); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(recordingResult(*taskDir, m))
}

func runRecordingScan(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("recording-scan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("output-root", "", "Voxly output root")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || !filepath.IsAbs(*root) {
		return fmt.Errorf("--output-root must be absolute")
	}
	entries, err := os.ReadDir(*root)
	if os.IsNotExist(err) {
		return json.NewEncoder(stdout).Encode([]recordingCommandResult{})
	}
	if err != nil {
		return err
	}
	results := make([]recordingCommandResult, 0)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		taskDir := filepath.Join(*root, entry.Name())
		m, err := LoadManifest(taskDir)
		if err != nil || m.SourceKind != "microphone_recording" || m.Recording == nil {
			continue
		}
		changed := false
		if m.Recording.Status == "recording" || m.Recording.Status == "paused" {
			m.Recording.Status = "interrupted"
			m.Recording.StoppedAt = nowFunc().UTC().Format(time.RFC3339)
			changed = true
		}
		checkpointFiles, _ := filepath.Glob(filepath.Join(taskDir, "audio", "checkpoints", "checkpoint_*.wav"))
		if len(checkpointFiles) > m.Recording.CheckpointCount {
			m.Recording.CheckpointCount = len(checkpointFiles)
			changed = true
		}
		if changed {
			if err := SaveManifest(taskDir, m); err != nil {
				continue
			}
		}
		results = append(results, recordingResult(taskDir, m))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].TaskID > results[j].TaskID })
	return json.NewEncoder(stdout).Encode(results)
}

func recordingResult(taskDir string, m *Manifest) recordingCommandResult {
	result := recordingCommandResult{TaskID: m.TaskID, TaskFolder: taskDir}
	if m.Recording != nil {
		result.Status, result.DurationMS, result.CheckpointCount = m.Recording.Status, m.Recording.DurationMS, m.Recording.CheckpointCount
	}
	result.CheckpointsDir = filepath.Join(taskDir, "audio", "checkpoints")
	if m.Recording != nil && m.Recording.FinalAudioReference != "" {
		result.AudioPath = filepath.Join(taskDir, filepath.FromSlash(m.Recording.FinalAudioReference))
	}
	return result
}
