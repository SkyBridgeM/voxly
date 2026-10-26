package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

func vadFixture(t *testing.T, silent bool) (string, *Manifest, string) {
	t.Helper()
	dir, err := CreateTaskDir(t.TempDir(), "vad")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "audio", "source.wav")
	data := append(pcmHeader(64000), make([]byte, 128000)...)
	for i := 44; i < len(data); i++ {
		data[i] = byte(i % 251)
	}
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	m := CreateManifest(dir, source, "transcript_only", 1, 10, "local-sensevoice", "sensevoice-small-int8")
	hash, _ := fileSHA256(source)
	m.Chunks.VAD = &taskmanifest.VADPlan{Version: 1, Ready: true, ModelSHA256: vadModelSHA, Parameters: vadParameters, Language: "zh", SourceSHA256: hash, TotalSamples: 64000}
	m.Chunks.Format = "wav"
	if !silent {
		m.Chunks.Items = []ChunkItem{
			{ID: "part_001", State: ChunkPending, StartSample: 16000, EndSample: 32000},
			{ID: "part_002", State: ChunkPending, StartSample: 48000, EndSample: 64000},
		}
	} else {
		m.Chunks.Items = []ChunkItem{}
	}
	m.Chunks.Total = len(m.Chunks.Items)
	if err := SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	return dir, m, source
}

func finishVADFixture(t *testing.T, dir string, m *Manifest) {
	t.Helper()
	if err := materializeVADWave(context.Background(), dir, m, m.Source.Path); err != nil {
		t.Fatal(err)
	}
	for i := range m.Chunks.Items {
		item := &m.Chunks.Items[i]
		if err := os.WriteFile(item.TranscriptPath, []byte("测试讲话。\n"), 0600); err != nil {
			t.Fatal(err)
		}
		item.TranscriptSHA256, _ = fileSHA256(item.TranscriptPath)
		item.State = ChunkCompleted
	}
	path := filepath.Join(dir, "transcript.md")
	if err := mergeTaskTranscript(dir, m, path); err != nil {
		t.Fatal(err)
	}
	if err := markArtifactReady(path, &m.Artifacts.Transcript); err != nil {
		t.Fatal(err)
	}
	if err := completeCanonicalTranscript(dir, path, m); err != nil {
		t.Fatal(err)
	}
	if err := MarkTaskCompleted(dir, m); err != nil {
		t.Fatal(err)
	}
}

func TestVADPlanMaterializationAndCancellation(t *testing.T) {
	dir, m, source := vadFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := materializeVADWave(ctx, dir, m, source); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	saved, err := LoadManifest(dir)
	if err != nil || !reflect.DeepEqual(saved.Chunks.VAD, m.Chunks.VAD) {
		t.Fatalf("plan lost: %v", err)
	}
	if err := materializeVADWave(context.Background(), dir, m, source); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(source)
	clip, _ := os.ReadFile(m.Chunks.Items[0].AudioPath)
	if !bytes.Equal(clip[44:], raw[44+32000:44+64000]) {
		t.Fatal("sample range was not exact")
	}
	m.Chunks.Items[1].StartSample = 100
	if err := validVADPlan(m); err == nil {
		t.Fatal("overlapping plan accepted")
	}
}

func TestVADCleanupRetainsTimingAndResumeRepairsResults(t *testing.T) {
	dir, m, _ := vadFixture(t, false)
	finishVADFixture(t, dir, m)
	before, _ := os.ReadFile(filepath.Join(dir, vadResults))
	p, err := taskCache(filepath.Dir(dir), filepath.Base(dir), "", false)
	if err != nil || !p.CanClean {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if _, err := taskCache(filepath.Dir(dir), filepath.Base(dir), p.Token, true); err != nil {
		t.Fatal(err)
	}
	m, err = LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !vadArtifactsValid(dir, m) {
		t.Fatal("cleanup invalidated timed results")
	}
	ReconcileChunks(dir, m)
	ReconcileArtifacts(dir, m)
	if _, done := ResumeStage(m); !done {
		t.Fatal("cleaned VAD task must remain completed")
	}
	if err := os.Remove(filepath.Join(dir, vadTimed)); err != nil {
		t.Fatal(err)
	}
	if stage, _ := ResumeStage(m); stage != TaskMerging {
		t.Fatalf("repair stage: %s", stage)
	}
	if err := executeMerging(dir, m); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, vadResults))
	if !bytes.Equal(before, after) || !vadArtifactsValid(dir, m) {
		t.Fatal("repair changed the fixed timeline")
	}
}

func TestVADCleanupBlocksUnverifiedTiming(t *testing.T) {
	dir, m, _ := vadFixture(t, false)
	finishVADFixture(t, dir, m)
	if err := os.WriteFile(filepath.Join(dir, vadResults), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := previewCacheLocked(dir, m)
	if err != nil || p.CanClean || p.Reason != "transcript_unverified" {
		t.Fatalf("unsafe preview %+v %v", p, err)
	}
	if !isCompleteFile(m.Chunks.Items[0].AudioPath) {
		t.Fatal("preview removed cache")
	}
}

func TestVADSilenceAndOwnedTemporaryCleanup(t *testing.T) {
	dir, m, _ := vadFixture(t, true)
	finishVADFixture(t, dir, m)
	if !vadArtifactsValid(dir, m) {
		t.Fatal("silent output missing")
	}
	if _, done := ResumeStage(m); !done {
		t.Fatal("silence task never completes")
	}
	work := filepath.Join(dir, ".vad-work")
	os.Mkdir(work, 0700)
	unknown := filepath.Join(work, "keep.txt")
	os.WriteFile(unknown, []byte("keep"), 0600)
	if err := cleanVADWork(dir); err == nil {
		t.Fatal("unknown work file deleted")
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatal("unknown work file lost")
	}
	os.Remove(unknown)
	os.WriteFile(filepath.Join(work, "audio.wav"), []byte("temporary"), 0600)
	if err := cleanVADWork(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(work); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned temporary directory not reclaimed")
	}
}
