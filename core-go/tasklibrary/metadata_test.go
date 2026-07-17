package tasklibrary

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

func TestUpdateMetadataPreservesExecutionArtifactsAndProvenance(t *testing.T) {
	root := t.TempDir()
	manifest := manifestFor("task-1", "completed", "2026-07-14T10:00:00Z", "transcript_and_generate")
	manifest.SchemaVersion = taskmanifest.LatestSchemaVersion
	manifest.RequestedTemplateID = "structured-notes"
	manifest.CanonicalTranscript = &taskmanifest.CanonicalTranscript{
		TranscriptID: "canonical", RevisionID: "tr-1", ArtifactReference: "transcript.md",
		ContentSHA256: "hash", HashContractVersion: 1, CreatedAt: "2026-07-14T08:00:00Z",
		ASRProvider: "mimo", ASRModel: "mimo-v2.5-asr", Status: "completed",
	}
	manifest.Generations = []taskmanifest.GenerationRecord{{
		GenerationID: "gen-1", TemplateID: "structured-notes", TemplateRevision: 1,
		TemplateDisplayName: "Structured notes", TemplateInstructions: "notes",
		CreatedAt: "2026-07-14T09:00:00Z", UpdatedAt: "2026-07-14T09:00:00Z",
		SourceTranscriptRevisionID: "tr-1", SourceTranscriptSHA256: "hash",
		LLMProvider: "deepseek", LLMModel: "deepseek-v4-flash",
		PromptCompositionVersion: 1, PromptHash: "prompt", Status: "completed",
		ArtifactReference: "generated/structured-notes/gen-1.md",
	}}
	manifest.UserMetadata = &taskmanifest.UserMetadata{Favorite: boolPointer(true), Annotations: []string{"keep"}}
	manifest.UnknownFields = map[string]json.RawMessage{"future": json.RawMessage(`{"keep":true}`)}
	manifest.UserMetadata.UnknownFields = map[string]json.RawMessage{"future_label": json.RawMessage(`"keep"`)}
	writeTask(t, root, "folder", manifest)
	path := filepath.Join(root, "folder", ManifestFilename)
	before, err := taskmanifest.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := updateMetadataAt(root, "task-1", MetadataUpdate{
		Title: "  Quarterly review  ", Description: "  Decisions and follow-ups  ",
		Tags: []string{"工作", "项目", "工作", " "},
	}, time.Date(2026, 7, 17, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Quarterly review" || got.Description != "Decisions and follow-ups" || !reflect.DeepEqual(got.Tags, []string{"工作", "项目"}) || got.UpdatedAt != "2026-07-17T03:04:05Z" {
		t.Fatalf("metadata = %#v", got)
	}
	after, err := taskmanifest.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Artifacts, after.Artifacts) || !reflect.DeepEqual(before.CanonicalTranscript, after.CanonicalTranscript) || !reflect.DeepEqual(before.Generations, after.Generations) {
		t.Fatal("metadata update changed artifacts or provider/generation history")
	}
	if after.UserMetadata.Favorite == nil || !*after.UserMetadata.Favorite || !reflect.DeepEqual(after.UserMetadata.Annotations, []string{"keep"}) {
		t.Fatal("metadata update erased existing user-owned fields")
	}
	if len(after.UnknownFields["future"]) == 0 || len(after.UserMetadata.UnknownFields["future_label"]) == 0 {
		t.Fatal("metadata update erased compatible unknown fields")
	}
}

func TestReadMetadataLegacyFallbackDoesNotUseFilename(t *testing.T) {
	root := t.TempDir()
	manifest := manifestFor("legacy", "completed", "2026-07-14T10:00:00Z", "transcript")
	manifest.Source.OriginalName = "private-recording.wav"
	writeTask(t, root, "legacy-folder", manifest)
	metadata, err := ReadMetadata(root, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Title != "Task 2026-07-14 08:00" || metadata.Description != "" || len(metadata.Tags) != 0 {
		t.Fatalf("legacy metadata = %#v", metadata)
	}
}

func TestMetadataSecurityRejectsInvalidMissingAndOutsideTasks(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "valid", manifestFor("valid", "completed", "2026-07-14T10:00:00Z", "transcript"))
	if _, err := UpdateMetadata(root, "../valid", MetadataUpdate{}); !errors.Is(err, ErrInvalidTaskID) {
		t.Fatalf("traversal error = %v", err)
	}
	if _, err := UpdateMetadata(root, "missing", MetadataUpdate{}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing error = %v", err)
	}

	outside := t.TempDir()
	writeTask(t, outside, "outside", manifestFor("outside", "completed", "2026-07-14T10:00:00Z", "transcript"))
	if err := os.Symlink(filepath.Join(outside, "outside"), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, filepath.Join(outside, "outside", ManifestFilename))
	if _, err := UpdateMetadata(root, "outside", MetadataUpdate{Title: "escape"}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("outside error = %v", err)
	}
	if after := readFile(t, filepath.Join(outside, "outside", ManifestFilename)); !reflect.DeepEqual(before, after) {
		t.Fatal("outside manifest changed")
	}
}

func TestMetadataValidation(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "valid", manifestFor("valid", "completed", "2026-07-14T10:00:00Z", "transcript"))
	if _, err := UpdateMetadata(root, "valid", MetadataUpdate{Title: string(make([]rune, MaxTitleLength+1))}); err == nil {
		t.Fatal("oversized title accepted")
	}
	tooMany := make([]string, MaxTags+1)
	for i := range tooMany {
		tooMany[i] = "tag"
	}
	if _, err := UpdateMetadata(root, "valid", MetadataUpdate{Tags: tooMany}); err == nil {
		t.Fatal("too many tags accepted")
	}
}

func TestUserMetadataJSONContainsStableFieldsAndNoCredentials(t *testing.T) {
	data, err := json.Marshal(taskmanifest.UserMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value["title"] != "" || value["description"] != "" || value["tags"] == nil || len(value) != 3 {
		t.Fatalf("UserMetadata JSON = %s", data)
	}
}

func boolPointer(value bool) *bool { return &value }

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
