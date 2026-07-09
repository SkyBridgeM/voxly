package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ApplyObsidianTemplate reads a template file and replaces placeholders.
// Supported placeholders (matching Python behavior):
//
//	{{ title }}           — safe_slug of the task name
//	{{ generated_at }}    — UTC timestamp "YYYY-MM-DD HH:MM UTC"
//	{{ generated_minutes }} — raw LLM output
//	{{ transcript }}      — full transcript.md content
//	{{ task_name }}       — task directory name
func ApplyObsidianTemplate(templatePath string, minutesText, title, taskName, transcript string) (string, error) {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("reading Obsidian template: %w", err)
	}

	generatedAt := time.Now().UTC().Format("2006-01-02 15:04 UTC")

	result := string(data)
	result = strings.ReplaceAll(result, "{{ title }}", title)
	result = strings.ReplaceAll(result, "{{ generated_at }}", generatedAt)
	result = strings.ReplaceAll(result, "{{ generated_minutes }}", minutesText)
	result = strings.ReplaceAll(result, "{{ transcript }}", transcript)
	result = strings.ReplaceAll(result, "{{ task_name }}", taskName)
	return result, nil
}

// ExtractTitle extracts a safe slug title from a task directory name.
// Task dir format: "YYYYMMDD_HHMMSS_slug" — returns the slug part.
func ExtractTitle(taskDirName string) string {
	parts := strings.SplitN(taskDirName, "_", 3)
	if len(parts) >= 3 {
		return SafeSlug(parts[2])
	}
	return SafeSlug(taskDirName)
}
