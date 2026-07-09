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
// a merged transcript.md to outputPath in the same format as the Python implementation.
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
		sections = append(sections, fmt.Sprintf("[Part %03d]\n\n%s", p.number, text))
	}

	output := strings.Join(sections, "\n\n") + "\n"
	return os.WriteFile(outputPath, []byte(output), 0644)
}
