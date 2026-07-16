package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var partRE = regexp.MustCompile(`^part_(\d+)\.txt$`)

// MergeTranscripts reads all part_*.txt files from transcriptsDir and writes
// the provider-neutral canonical transcript. Chunking is visible to the
// runtime, not to the reader, so part identities never enter transcript.md.
func MergeTranscripts(transcriptsDir, outputPath string) error {
	entries, err := os.ReadDir(transcriptsDir)
	if err != nil {
		return fmt.Errorf("reading transcripts dir: %w", err)
	}

	type part struct {
		number int
		path   string
	}

	var parts []part
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := partRE.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		num, _ := strconv.Atoi(match[1])
		parts = append(parts, part{number: num, path: filepath.Join(transcriptsDir, e.Name())})
	}

	if len(parts) == 0 {
		return fmt.Errorf("no valid transcript part files found in %s", transcriptsDir)
	}

	sort.Slice(parts, func(i, j int) bool { return parts[i].number < parts[j].number })

	var sections []string
	for _, p := range parts {
		data, err := os.ReadFile(p.path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Base(p.path), err)
		}
		text := strings.TrimSpace(string(data))
		if text != "" {
			sections = append(sections, text)
		}
	}

	output := strings.Join(sections, "\n\n")
	if output != "" {
		output += "\n"
	}
	return WriteFileAtomically(outputPath, []byte(output), 0600)
}
