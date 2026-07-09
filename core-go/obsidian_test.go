package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyObsidianTemplateSnapshot(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "template.md")

	template := `---
title: {{ title }}
date: {{ generated_at }}
task: {{ task_name }}
---

{{ generated_minutes }}

---

## Full Transcript

{{ transcript }}
`
	os.WriteFile(templatePath, []byte(template), 0644)

	got, err := ApplyObsidianTemplate(
		templatePath,
		"## Summary\n\nKey points.",
		"Q4_planning",
		"20260709_143022_Q4_planning",
		"[Part 001]\n\nHello.",
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "title: Q4_planning") {
		t.Error("missing title placeholder replacement")
	}
	if !strings.Contains(got, "task: 20260709_143022_Q4_planning") {
		t.Error("missing task_name placeholder replacement")
	}
	if !strings.Contains(got, "## Summary") {
		t.Error("missing generated_minutes replacement")
	}
	if !strings.Contains(got, "[Part 001]") {
		t.Error("missing transcript replacement")
	}
	if !strings.Contains(got, "date: ") {
		t.Error("missing generated_at replacement")
	}
}

func TestApplyObsidianTemplateMissingFile(t *testing.T) {
	_, err := ApplyObsidianTemplate("/nonexistent/template.md", "minutes", "title", "task", "transcript")
	if err == nil {
		t.Fatal("expected error for missing template file")
	}
	if !strings.Contains(err.Error(), "Obsidian template") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApplyObsidianTemplateNoPlaceholders(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "simple.md")
	os.WriteFile(templatePath, []byte("Just plain text."), 0644)

	got, err := ApplyObsidianTemplate(templatePath, "minutes", "title", "task", "transcript")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Just plain text." {
		t.Errorf("got %q, want %q", got, "Just plain text.")
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"20260709_143022_Q4_planning", "Q4_planning"},
		{"20260709_143022_meeting_notes", "meeting_notes"},
		{"simple_name", "simple_name"},
		{"20260709_143022", "20260709_143022"},
		{"", "meeting"},
	}
	for _, tt := range tests {
		got := ExtractTitle(tt.input)
		if got != tt.want {
			t.Errorf("ExtractTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
