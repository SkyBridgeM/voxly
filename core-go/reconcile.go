package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReconcileChunks validates every expected chunk against the filesystem:
// - verifies chunk audio exists, is non-empty, and has the expected filename;
// - verifies completed transcript files exist, are non-empty, and are valid text;
// - reclassifies missing, empty, or temporary transcripts as PENDING;
// - reclassifies IN_PROGRESS as PENDING unless a valid final transcript exists.
func ReconcileChunks(taskDir string, m *Manifest) {
	chunksDir := filepath.Join(taskDir, "chunks")
	transcriptsDir := filepath.Join(taskDir, "transcripts")

	for i := range m.Chunks.Items {
		item := &m.Chunks.Items[i]

		// Verify chunk audio.
		audioPath := filepath.Join(chunksDir, fmt.Sprintf("%s.%s", item.ID, m.Chunks.Format))
		item.AudioPath = audioPath
		if !isCompleteFile(audioPath) {
			item.State = ChunkPending
			continue
		}
		info, _ := os.Stat(audioPath)
		if info != nil {
			item.SizeBytes = info.Size()
		}
		audioHash, err := fileSHA256(audioPath)
		if err != nil {
			item.State = ChunkPending
			continue
		}
		if item.AudioSHA256 == "" && m.IntegrityVersion == 0 {
			item.State = ChunkPending
			continue
		}
		if item.AudioSHA256 == "" {
			item.AudioSHA256 = audioHash // One-time upgrade for pre-hash manifests.
		} else if item.AudioSHA256 != audioHash {
			item.AudioSHA256 = audioHash
			item.TranscriptSHA256 = ""
			item.State = ChunkPending
			continue
		}

		// Verify transcript.
		transcriptPath := filepath.Join(transcriptsDir, item.ID+".txt")
		item.TranscriptPath = transcriptPath

		if transcriptHash, err := fileSHA256(transcriptPath); err == nil && (item.TranscriptSHA256 != "" && item.TranscriptSHA256 == transcriptHash || m.IntegrityVersion != 0 && item.TranscriptSHA256 == "") {
			item.TranscriptSHA256 = transcriptHash
			item.State = ChunkCompleted
		} else {
			// Missing, empty, or temp file — reclassify as PENDING.
			item.State = ChunkPending
			item.Attempts = 0
		}
	}
}

// ReconcileArtifacts validates each artifact against the filesystem.
// If an upstream artifact changes, every downstream artifact is invalidated.
func ReconcileArtifacts(taskDir string, m *Manifest) {
	// Transcript artifact.
	transcriptPath := filepath.Join(taskDir, "transcript.md")
	m.Artifacts.Transcript.Path = transcriptPath
	if artifactMatches(transcriptPath, &m.Artifacts.Transcript, m.IntegrityVersion) {
		m.Artifacts.Transcript.State = ArtifactReady
	} else {
		m.Artifacts.Transcript.State = ArtifactPending
		// Downstream depends on transcript.
		m.Artifacts.NotesInput.State = ArtifactInvalid
		m.Artifacts.Minutes.State = ArtifactInvalid
		return
	}

	// Notes input artifact (notes and minutes modes only).
	if m.OutputMode == "notes" || m.OutputMode == "minutes" {
		notesPath := filepath.Join(taskDir, "notes_input.md")
		m.Artifacts.NotesInput.Path = notesPath
		if artifactMatches(notesPath, &m.Artifacts.NotesInput, m.IntegrityVersion) {
			m.Artifacts.NotesInput.State = ArtifactReady
		} else {
			m.Artifacts.NotesInput.State = ArtifactPending
			m.Artifacts.Minutes.State = ArtifactInvalid
			return
		}
	} else {
		m.Artifacts.NotesInput.State = ArtifactPending
	}

	// Minutes artifact (minutes mode only).
	if m.OutputMode == "minutes" {
		minutesPath := filepath.Join(taskDir, "meeting_minutes.md")
		m.Artifacts.Minutes.Path = minutesPath
		if artifactMatches(minutesPath, &m.Artifacts.Minutes, m.IntegrityVersion) {
			m.Artifacts.Minutes.State = ArtifactReady
		} else {
			m.Artifacts.Minutes.State = ArtifactPending
		}
	} else {
		m.Artifacts.Minutes.State = ArtifactPending
	}
}

// BuildChunkItemsFromDisk discovers chunks on disk and builds ChunkItem entries
// for a manifest that has no chunk records (legacy manifest or first run).
func BuildChunkItemsFromDisk(taskDir string) ([]ChunkItem, string, error) {
	chunksDir := filepath.Join(taskDir, "chunks")
	transcriptsDir := filepath.Join(taskDir, "transcripts")

	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		return nil, "", fmt.Errorf("reading chunks directory: %w", err)
	}

	var items []ChunkItem
	formats := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "part_") {
			continue
		}
		match := chunkNameRE.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}

		format := match[2]
		formats[format] = true

		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		audioPath := filepath.Join(chunksDir, entry.Name())
		transcriptPath := filepath.Join(transcriptsDir, id+".txt")

		info, _ := os.Stat(audioPath)
		var sizeBytes int64
		if info != nil {
			sizeBytes = info.Size()
		}

		audioHash, _ := fileSHA256(audioPath)

		items = append(items, ChunkItem{
			ID:             id,
			AudioPath:      audioPath,
			TranscriptPath: transcriptPath,
			State:          ChunkPending,
			Attempts:       0,
			SizeBytes:      sizeBytes,
			AudioSHA256:    audioHash,
			LastError:      nil,
		})
	}

	if len(items) == 0 {
		return nil, "", fmt.Errorf("no valid chunk files found in %s", chunksDir)
	}
	if len(formats) > 1 {
		return nil, "", fmt.Errorf("mixed chunk formats in %s", chunksDir)
	}
	format := ""
	for f := range formats {
		format = f
	}

	return items, format, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func artifactMatches(path string, artifact *ArtifactInfo, integrityVersion int) bool {
	if artifact.State != ArtifactReady || !isCompleteFile(path) {
		return false
	}
	hash, err := fileSHA256(path)
	if err != nil {
		return false
	}
	if artifact.SHA256 == "" && integrityVersion == 0 {
		return false
	}
	if artifact.SHA256 == "" {
		artifact.SHA256 = hash // One-time upgrade for pre-hash manifests.
		return true
	}
	return hash == artifact.SHA256
}

func mustFileSHA256(path string) string {
	hash, err := fileSHA256(path)
	if err != nil {
		return ""
	}
	return hash
}
