package main

import (
	"fmt"
	"os"
	"strings"
)

// CreateNotesInput concatenates the prompt and transcript into notes_input.md.
// Format matches Python's create_notes_input():
//
//	<prompt>
//
//	---
//
//	## Transcript
//
//	<transcript>
func CreateNotesInput(promptPath, transcriptPath, outputPath string) error {
	promptData, err := os.ReadFile(promptPath)
	if err != nil {
		return fmt.Errorf("reading prompt file: %w", err)
	}

	transcriptData, err := os.ReadFile(transcriptPath)
	if err != nil {
		return fmt.Errorf("reading transcript file: %w", err)
	}

	prompt := strings.TrimSpace(string(promptData))
	transcript := strings.TrimSpace(string(transcriptData))

	content := fmt.Sprintf("%s\n\n---\n\n## Transcript\n\n%s\n", prompt, transcript)

	return WriteFileAtomically(outputPath, []byte(content), 0600)
}
