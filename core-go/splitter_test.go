package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestCalculateRetryChunkSeconds(t *testing.T) {
	tests := []struct {
		initial int
		want    []int
	}{
		{300, []int{300, 150, 75, 60}},
		{120, []int{120, 60}},
		{60, []int{60}},
		{30, []int{30}}, // below min, single value
		{10, []int{10}},
		{240, []int{240, 120, 60}},
	}
	for _, tt := range tests {
		got := CalculateRetryChunkSeconds(tt.initial)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("CalculateRetryChunkSeconds(%d) = %v, want %v", tt.initial, got, tt.want)
		}
	}
}

func TestChunkMinutesToMillisecondsRejectsOverflow(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("overflow input requires a 64-bit int")
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	overflowMinutes := int(maxInt64/(60*1000) + 1)
	if _, err := chunkMinutesToMilliseconds(overflowMinutes); err == nil {
		t.Fatal("expected chunk-duration overflow error")
	}
	if _, err := chunkMinutesToMilliseconds(-1); err == nil {
		t.Fatal("expected negative chunk-duration error")
	}
}

func TestClearChunks_ReportsDirectoryError(t *testing.T) {
	root := t.TempDir()
	chunksPath := filepath.Join(root, "chunks")
	if err := os.WriteFile(chunksPath, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := clearChunks(chunksPath, "wav"); err == nil {
		t.Fatal("expected chunk cleanup error")
	}
}

func TestChunkAudioFormat(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mp3", "mp3"},
		{"wav", "wav"},
		{"m4a", "wav"},
	}
	for _, tt := range tests {
		got := chunkAudioFormat(tt.input, "source")
		if got != tt.want {
			t.Errorf("chunkAudioFormat(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
