package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mergeFixture(t *testing.T, parts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	transcriptsDir := filepath.Join(dir, "transcripts")
	if err := os.MkdirAll(transcriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, text := range parts {
		if err := os.WriteFile(filepath.Join(transcriptsDir, name), []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	outputPath := filepath.Join(dir, "transcript.md")
	if err := MergeTranscripts(transcriptsDir, outputPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMergeTranscriptsCanonicalOrderWithoutMarkers(t *testing.T) {
	got := mergeFixture(t, map[string]string{
		"part_003.txt": "Third part text\n",
		"part_001.txt": "First part text\n",
		"part_002.txt": "Second part text\n",
	})
	want := "First part text\n\nSecond part text\n\nThird part text\n"
	if got != want {
		t.Fatalf("canonical transcript mismatch\ngot:  %q\nwant: %q", got, want)
	}
	if strings.Contains(got, "[Part 001]") {
		t.Fatal("canonical transcript contains a generated part marker")
	}
}

func TestMergeTranscriptsPreservesLegitimatePartText(t *testing.T) {
	got := mergeFixture(t, map[string]string{
		"part_001.txt": "The speaker literally said [part 001] during the recording.",
	})
	want := "The speaker literally said [part 001] during the recording.\n"
	if got != want {
		t.Fatalf("provider text was rewritten\ngot:  %q\nwant: %q", got, want)
	}
}

func TestMergeTranscriptsWhitespaceAndEmptyParts(t *testing.T) {
	tests := []struct {
		name  string
		parts map[string]string
		want  string
	}{
		{
			name:  "one non-empty part",
			parts: map[string]string{"part_001.txt": "  text with spaces  \n\n"},
			want:  "text with spaces\n",
		},
		{
			name: "empty parts between valid parts",
			parts: map[string]string{
				"part_001.txt": "first\n\n",
				"part_002.txt": "  \n\t",
				"part_003.txt": "\nsecond\n",
			},
			want: "first\n\nsecond\n",
		},
		{
			name: "all successful parts empty",
			parts: map[string]string{
				"part_001.txt": "",
				"part_002.txt": " \n\t",
			},
			want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mergeFixture(t, test.parts); got != test.want {
				t.Fatalf("canonical transcript mismatch\ngot:  %q\nwant: %q", got, test.want)
			}
		})
	}
}

func TestMergeTranscriptsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	transcriptsDir := filepath.Join(dir, "empty")
	if err := os.MkdirAll(transcriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	err := MergeTranscripts(transcriptsDir, filepath.Join(dir, "out.md"))
	if err == nil || !strings.Contains(err.Error(), "no valid transcript part files") {
		t.Fatalf("expected no-parts error, got %v", err)
	}
}

func TestMergeTranscriptsNoPartsDir(t *testing.T) {
	err := MergeTranscripts(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "out.md"))
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}
