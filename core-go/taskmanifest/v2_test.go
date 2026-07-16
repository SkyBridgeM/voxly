package taskmanifest

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestV2ManifestRoundTrip(t *testing.T) {
	retryable := true
	m := &TaskManifest{
		SchemaVersion: CurrentSchemaVersion, TaskID: "task-v2",
		CreatedAt: "2026-07-16T00:00:00Z", UpdatedAt: "2026-07-16T00:01:00Z",
		Backend: "go", Provider: "mimo", OutputMode: "transcript_and_generate",
		RequestedTemplateID: "structured-notes", Status: "completed",
		Source: SourceInfo{Path: "audio/source.wav"}, SourceOwnership: "task_copy", InputFormat: "wav",
		Artifacts:           Artifacts{Transcript: Artifact{State: "completed", Path: "transcript.md"}, NotesInput: Artifact{State: "unavailable"}, Minutes: Artifact{State: "unavailable"}},
		CanonicalTranscript: &CanonicalTranscript{TranscriptID: "canonical", RevisionID: "tr_1", ArtifactReference: "transcript.md", ContentSHA256: "abc", HashContractVersion: 1, CreatedAt: "2026-07-16T00:00:30Z", ASRProvider: "mimo", ASRModel: "m", Status: "completed"},
		Generations:         []GenerationRecord{{GenerationID: "gen_1", TemplateID: "structured-notes", TemplateRevision: 1, TemplateDisplayName: "Structured notes", TemplateInstructions: "notes", CreatedAt: "2026-07-16T00:00:40Z", UpdatedAt: "2026-07-16T00:01:00Z", SourceTranscriptRevisionID: "tr_1", SourceTranscriptSHA256: "abc", LLMProvider: "deepseek", LLMModel: "model", PromptCompositionVersion: 1, PromptHash: "hash", Status: "failed", Error: &ArtifactError{Code: "generation_failed", Message: "safe", Retryable: &retryable}}},
	}
	path := filepath.Join(t.TempDir(), ManifestFilename)
	if err := WriteManifest(path, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Generations) != 1 || loaded.CanonicalTranscript.RevisionID != "tr_1" || loaded.RequestedTemplateID != "structured-notes" {
		t.Fatalf("round trip lost V2 data: %#v", loaded)
	}
}

func TestV2FixturesValidate(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	pattern := filepath.Join(filepath.Dir(filename), "..", "..", "docs", "fixtures", "task_manifest", "task_manifest_v2_*.json")
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) != 3 {
		t.Fatalf("fixtures = %v, err = %v", paths, err)
	}
	for _, path := range paths {
		if _, err := LoadManifest(path); err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
	}
}

func TestV2RejectsUnsafeGenerationReference(t *testing.T) {
	retryable := false
	record := GenerationRecord{GenerationID: "gen_safe", TemplateID: "unknown-template", TemplateRevision: 1, TemplateDisplayName: "Unknown", TemplateInstructions: "safe", CreatedAt: "2026-07-16T00:00:00Z", UpdatedAt: "2026-07-16T00:00:00Z", SourceTranscriptRevisionID: "tr_1", SourceTranscriptSHA256: "hash", LLMProvider: "deepseek", LLMModel: "model", PromptCompositionVersion: 1, PromptHash: "hash", Status: "failed", ArtifactReference: "../escape.md", Error: &ArtifactError{Code: "failed", Message: "safe", Retryable: &retryable}}
	if err := record.validate(); err == nil {
		t.Fatal("unsafe generation reference accepted")
	}
	record.ArtifactReference = ""
	if err := record.validate(); err != nil {
		t.Fatalf("unknown template should remain readable: %v", err)
	}
}
