package main

import (
	"reflect"
	"testing"
)

func TestReorderArgs_FlagsAfterPositional(t *testing.T) {
	input := []string{"input.mp3", "--chunk-minutes", "3", "--output-dir", "./out"}
	want := []string{"--chunk-minutes", "3", "--output-dir", "./out", "input.mp3"}
	got := reorderArgs(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs(%v) = %v, want %v", input, got, want)
	}
}

func TestReorderArgs_FlagsBeforePositional(t *testing.T) {
	input := []string{"--chunk-minutes", "3", "input.mp3", "--output-dir", "./out"}
	want := []string{"--chunk-minutes", "3", "--output-dir", "./out", "input.mp3"}
	got := reorderArgs(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs(%v) = %v, want %v", input, got, want)
	}
}

func TestReorderArgs_OnlyFlags(t *testing.T) {
	input := []string{"--chunk-minutes", "3", "--output-dir", "./out"}
	want := []string{"--chunk-minutes", "3", "--output-dir", "./out"}
	got := reorderArgs(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs(%v) = %v, want %v", input, got, want)
	}
}

func TestReorderArgs_OnlyPositional(t *testing.T) {
	input := []string{"input.mp3"}
	want := []string{"input.mp3"}
	got := reorderArgs(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs(%v) = %v, want %v", input, got, want)
	}
}

func TestReorderArgs_Empty(t *testing.T) {
	got := reorderArgs(nil)
	if len(got) != 0 {
		t.Errorf("reorderArgs(nil) = %v, want empty", got)
	}
}

func TestReorderArgs_BoolFlag(t *testing.T) {
	input := []string{"input.mp3", "--verbose"}
	want := []string{"--verbose", "input.mp3"}
	got := reorderArgs(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs(%v) = %v, want %v", input, got, want)
	}
}

func TestReorderArgs_MixedOrder(t *testing.T) {
	input := []string{"--output-mode", "transcript", "test.mp3", "--chunk-minutes", "5", "--max-upload-mb", "20"}
	want := []string{"--output-mode", "transcript", "--chunk-minutes", "5", "--max-upload-mb", "20", "test.mp3"}
	got := reorderArgs(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs(%v) = %v, want %v", input, got, want)
	}
}

func TestReorderArgsCleanupBooleanAndEqualsBeforeInput(t *testing.T) {
	for _, args := range [][]string{
		{"--auto-clean-chunks", "input.wav", "--output-dir", "out"},
		{"--auto-clean-chunks=true", "input.wav", "--output-dir", "out"},
		{"--control-stdin", "input.wav", "--output-dir", "out"},
	} {
		reordered := reorderArgs(args)
		if reordered[len(reordered)-1] != "input.wav" || reordered[len(reordered)-2] != "out" {
			t.Fatalf("boolean option swallowed input: %v", reordered)
		}
	}
}
