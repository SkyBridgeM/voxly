package tasklibrary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

func TestDiscoverMultipleTasksDeterministicOrdering(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "folder-b", manifestFor("task-b", "completed", "2026-07-14T09:00:00Z", "transcript"))
	writeTask(t, root, "folder-c", manifestFor("task-c", "completed", "2026-07-14T10:00:00Z", "transcript"))
	writeTask(t, root, "folder-a", manifestFor("task-a", "completed", "2026-07-14T09:00:00Z", "transcript"))

	result, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{result.Tasks[0].TaskID, result.Tasks[1].TaskID, result.Tasks[2].TaskID}
	want := []string{"task-c", "task-a", "task-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("task order = %v, want %v", got, want)
	}
}

func TestDiscoverOrderingFallbacks(t *testing.T) {
	t.Run("equal timestamps use task ID then folder", func(t *testing.T) {
		root := t.TempDir()
		writeTask(t, root, "folder-z", manifestFor("same", "completed", "2026-07-14T10:00:00Z", "transcript"))
		writeTask(t, root, "folder-a", manifestFor("same", "completed", "2026-07-14T10:00:00Z", "transcript"))
		writeTask(t, root, "folder-m", manifestFor("another", "completed", "2026-07-14T10:00:00Z", "transcript"))

		result, err := Discover(root)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, len(result.Tasks))
		for i, task := range result.Tasks {
			got[i] = task.TaskID + "/" + task.TaskFolder
		}
		want := []string{"another/folder-m", "same/folder-a", "same/folder-z"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("task order = %v, want %v", got, want)
		}
	})

	t.Run("malformed timestamps remain deterministic", func(t *testing.T) {
		root := t.TempDir()
		writeTask(t, root, "folder-a", manifestFor("task-a", "completed", "not-a-time", "transcript"))
		writeTask(t, root, "folder-b", manifestFor("task-b", "completed", "also-not-a-time", "transcript"))

		first, err := Discover(root)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Discover(root)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first.Tasks, second.Tasks) {
			t.Fatalf("malformed timestamp ordering changed: first=%#v second=%#v", first.Tasks, second.Tasks)
		}
	})
}

func TestSafeRelativeReferenceContainment(t *testing.T) {
	taskDir := filepath.Join(t.TempDir(), "task")
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "nested relative path", path: "outputs/final/transcript.md", want: "outputs/final/transcript.md"},
		{name: "outside absolute path", path: filepath.Join(filepath.Dir(taskDir), "outside.md"), want: ""},
		{name: "parent traversal", path: "../outside.md", want: ""},
		{name: "task directory itself", path: taskDir, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeRelativeReference(taskDir, tt.path); got != tt.want {
				t.Fatalf("safeRelativeReference(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIssueForPermissionFailure(t *testing.T) {
	issue := issueFor("task-folder", fmt.Errorf("wrapped: %w", os.ErrPermission))
	if issue.Code != IssueUnreadableManifest || issue.Message != "Task manifest cannot be read." {
		t.Fatalf("issueFor(permission) = %#v", issue)
	}
}

func TestTaskTitleUsesFilenameThenTimestampFallback(t *testing.T) {
	manifest := manifestFor("task-id", "completed", "2026-07-14T10:00:00Z", "transcript")
	manifest.Source.OriginalName = "  lecture.wav  "
	if title, source := taskTitle(manifest); title != "lecture" || source != "filename" {
		t.Fatalf("taskTitle() = %q (%s), want lecture (filename)", title, source)
	}
	manifest.Source.OriginalName = ""
	if title, source := taskTitle(manifest); title != "Task 2026-07-14 08:00" || source != "fallback" {
		t.Fatalf("taskTitle() = %q (%s), want timestamp fallback", title, source)
	}
}

func TestDiscoverLifecycleAndArtifactPresentation(t *testing.T) {
	root := t.TempDir()
	completed := manifestFor("completed", "completed", "2026-07-14T10:00:00Z", "minutes")
	completed.UserMetadata = &taskmanifest.UserMetadata{Title: "Research interview", Tags: []string{"访谈", "工作"}}
	writeTask(t, root, "completed-folder", completed)

	partial := manifestFor("partial", "completed_with_errors", "2026-07-14T09:00:00Z", "minutes")
	retryable := true
	partial.Artifacts.Minutes = taskmanifest.Artifact{
		State: "failed",
		Error: &taskmanifest.ArtifactError{Code: "MINUTES_GENERATING", Message: "provider unavailable", Retryable: &retryable},
	}
	writeTask(t, root, "partial-folder", partial)

	failed := manifestFor("failed", "failed", "2026-07-14T08:00:00Z", "transcript")
	retryable = false
	failed.Artifacts.Transcript = taskmanifest.Artifact{
		State: "failed",
		Error: &taskmanifest.ArtifactError{Code: "MERGING", Message: "merge failed", Retryable: &retryable},
	}
	writeTask(t, root, "failed-folder", failed)

	result, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 3 || len(result.Issues) != 0 {
		t.Fatalf("Discover() = %d tasks, %d issues", len(result.Tasks), len(result.Issues))
	}

	completedSummary := result.Tasks[0]
	if completedSummary.Title != "Research interview" || completedSummary.TitleSource != "user" {
		t.Errorf("completed title = %q (%s)", completedSummary.Title, completedSummary.TitleSource)
	}
	if !reflect.DeepEqual(completedSummary.Tags, []string{"访谈", "工作"}) {
		t.Errorf("tags = %v", completedSummary.Tags)
	}
	assertArtifactKinds(t, completedSummary.PrimaryArtifacts, []string{"minutes", "notes", "transcript"})
	for _, artifact := range completedSummary.PrimaryArtifacts {
		if artifact.Status != "completed" || artifact.Reference == "" || filepath.IsAbs(artifact.Reference) {
			t.Errorf("completed artifact = %#v", artifact)
		}
	}

	partialSummary := result.Tasks[1]
	if !partialSummary.HasPartialErrors || partialSummary.Status != "completed_with_errors" {
		t.Errorf("partial summary = %#v", partialSummary)
	}
	minutes := partialSummary.PrimaryArtifacts[0]
	if minutes.Status != "failed" || !minutes.HasError || minutes.Retryable == nil || !*minutes.Retryable {
		t.Errorf("failed minutes = %#v", minutes)
	}
	if partialSummary.PrimaryArtifacts[1].Status != "completed" || partialSummary.PrimaryArtifacts[2].Status != "completed" {
		t.Error("partial success hid completed outputs")
	}

	failedSummary := result.Tasks[2]
	if failedSummary.Status != "failed" || failedSummary.HasPartialErrors {
		t.Errorf("failed summary = %#v", failedSummary)
	}
	if failedSummary.PrimaryArtifacts[0].Status != "failed" || failedSummary.PrimaryArtifacts[0].Retryable == nil || *failedSummary.PrimaryArtifacts[0].Retryable {
		t.Errorf("failed transcript = %#v", failedSummary.PrimaryArtifacts[0])
	}
}

func TestDiscoverIsolatesMalformedAndUnsupportedManifests(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "valid", manifestFor("valid", "completed", "2026-07-14T10:00:00Z", "transcript"))

	malformedDir := filepath.Join(root, "malformed")
	if err := os.Mkdir(malformedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformedDir, ManifestFilename), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	unsupportedDir := filepath.Join(root, "unsupported")
	writeTask(t, root, "unsupported", manifestFor("unsupported", "completed", "2026-07-14T09:00:00Z", "transcript"))
	path := filepath.Join(unsupportedDir, ManifestFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"schema_version": "1.0"`, `"schema_version": "99.0"`, 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 1 || result.Tasks[0].TaskID != "valid" {
		t.Fatalf("valid tasks = %#v", result.Tasks)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	wantCodes := map[string]string{"malformed": IssueInvalidManifest, "unsupported": IssueUnsupportedSchema}
	for _, issue := range result.Issues {
		if issue.Code != wantCodes[issue.TaskFolder] {
			t.Errorf("issue = %#v", issue)
		}
		if strings.Contains(issue.Message, root) || strings.Contains(issue.Message, "not-json") {
			t.Errorf("issue leaked filesystem/content details: %#v", issue)
		}
	}
}

func TestDiscoverIgnoresDirectoriesWithoutManifestAndDoesNotRecurse(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "container", "nested-task")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := taskmanifest.WriteManifest(filepath.Join(nested, ManifestFilename), manifestFor("nested", "completed", "2026-07-14T10:00:00Z", "transcript")); err != nil {
		t.Fatal(err)
	}

	result, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 0 || len(result.Issues) != 0 {
		t.Fatalf("Discover() = %#v, want empty result", result)
	}
}

func TestSummaryDoesNotExposeSensitiveOrInternalFields(t *testing.T) {
	root := t.TempDir()
	manifest := manifestFor("safe", "completed", "2026-07-14T10:00:00Z", "transcript")
	manifest.Source.Path = "/Users/example/private/source.wav"
	manifest.Backend = "internal-backend"
	manifest.Provider = "internal-provider"
	manifest.Artifacts.Transcript.Path = "/Users/example/private/transcript.md"
	manifest.Chunks.Items = []taskmanifest.ChunkItem{{ID: "part_001", AudioPath: "/tmp/private.wav", State: "COMPLETED"}}
	manifest.Chunks.Total = 1
	writeTask(t, root, "safe-folder", manifest)

	result, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.Tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"/Users/example", "internal-backend", "internal-provider", "/tmp/private.wav", "unknown_fields", "checkpoint"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("summary exposed %q: %s", forbidden, text)
		}
	}
	if result.Tasks[0].PrimaryArtifacts[0].Reference != "" {
		t.Errorf("unsafe external artifact reference was exposed: %#v", result.Tasks[0].PrimaryArtifacts[0])
	}
}

func TestDiscoverDoesNotMutateFilesystem(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "task", manifestFor("task", "completed", "2026-07-14T10:00:00Z", "transcript"))
	before := snapshotTree(t, root)

	if _, err := Discover(root); err != nil {
		t.Fatal(err)
	}
	after := snapshotTree(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("filesystem changed:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestDiscoverV2GroupsGeneratedVersionsAndPreservesFailures(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "v2")
	if err := os.MkdirAll(filepath.Join(taskDir, "generated", "structured-notes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "transcript.md"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "generated", "structured-notes", "gen_1.md"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	retryable := true
	m := manifestFor("v2", "completed_with_errors", "2026-07-16T02:00:00Z", "transcript_and_generate")
	m.SchemaVersion = taskmanifest.LatestSchemaVersion
	m.RequestedTemplateID = "structured-notes"
	m.Artifacts.Transcript.Path = "transcript.md"
	m.CanonicalTranscript = &taskmanifest.CanonicalTranscript{TranscriptID: "canonical", RevisionID: "tr_1", ArtifactReference: "transcript.md", ContentSHA256: "hash", HashContractVersion: 1, CreatedAt: "2026-07-16T01:00:00Z", ASRProvider: "mimo", ASRModel: "model", Status: "completed"}
	m.Generations = []taskmanifest.GenerationRecord{
		{GenerationID: "gen_1", TemplateID: "structured-notes", TemplateRevision: 1, TemplateDisplayName: "Structured notes", TemplateInstructions: "notes", CreatedAt: "2026-07-16T01:10:00Z", UpdatedAt: "2026-07-16T01:10:00Z", SourceTranscriptRevisionID: "tr_1", SourceTranscriptSHA256: "hash", LLMProvider: "deepseek", LLMModel: "model", PromptCompositionVersion: 1, PromptHash: "one", Status: "completed", ArtifactReference: "generated/structured-notes/gen_1.md"},
		{GenerationID: "gen_2", TemplateID: "structured-notes", TemplateRevision: 1, TemplateDisplayName: "Structured notes", TemplateInstructions: "notes", CreatedAt: "2026-07-16T01:20:00Z", UpdatedAt: "2026-07-16T01:20:00Z", SourceTranscriptRevisionID: "tr_1", SourceTranscriptSHA256: "hash", LLMProvider: "deepseek", LLMModel: "model", PromptCompositionVersion: 1, PromptHash: "two", Status: "failed", Error: &taskmanifest.ArtifactError{Code: "generation_failed", Message: "safe", Retryable: &retryable}},
	}
	writeTask(t, root, "v2", m)
	result, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 1 || !result.Tasks[0].CanGenerate || len(result.Tasks[0].GeneratedContent) != 1 {
		t.Fatalf("summary = %#v", result.Tasks)
	}
	group := result.Tasks[0].GeneratedContent[0]
	if group.Latest == nil || group.Latest.GenerationID != "gen_1" || len(group.Versions) != 2 || group.Versions[0].Status != "failed" {
		t.Fatalf("group = %#v", group)
	}
	if result.Tasks[0].CanonicalTranscript.Provider != "mimo" || result.Tasks[0].CanonicalTranscript.Model != "model" {
		t.Fatalf("transcript provenance = %#v", result.Tasks[0].CanonicalTranscript)
	}
	if group.Latest.Provider != "deepseek" || group.Latest.Model != "model" {
		t.Fatalf("generation provenance = %#v", group.Latest)
	}
}

func manifestFor(taskID, status, updatedAt, outputMode string) *taskmanifest.TaskManifest {
	return &taskmanifest.TaskManifest{
		SchemaVersion:   taskmanifest.LegacySchemaVersion,
		TaskID:          taskID,
		CreatedAt:       "2026-07-14T08:00:00Z",
		UpdatedAt:       updatedAt,
		Backend:         "go",
		Provider:        "mimo",
		OutputMode:      outputMode,
		Status:          status,
		State:           "COMPLETED",
		Attempt:         1,
		Source:          taskmanifest.SourceInfo{Path: "audio/source.wav", OriginalName: "source.wav"},
		SourceOwnership: "task_copy",
		InputFormat:     "wav",
		Artifacts: taskmanifest.Artifacts{
			Transcript: taskmanifest.Artifact{State: "completed", Path: "transcript.md"},
			NotesInput: taskmanifest.Artifact{State: requestedState(outputMode == "notes" || outputMode == "minutes"), Path: requestedPath(outputMode == "notes" || outputMode == "minutes", "notes_input.md")},
			Minutes:    taskmanifest.Artifact{State: requestedState(outputMode == "minutes"), Path: requestedPath(outputMode == "minutes", "meeting_minutes.md")},
		},
	}
}

func requestedState(requested bool) string {
	if requested {
		return "completed"
	}
	return "unavailable"
}

func requestedPath(requested bool, path string) string {
	if requested {
		return path
	}
	return ""
}

func writeTask(t *testing.T, root, folder string, manifest *taskmanifest.TaskManifest) {
	t.Helper()
	dir := filepath.Join(root, folder)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := taskmanifest.WriteManifest(filepath.Join(dir, ManifestFilename), manifest); err != nil {
		t.Fatal(err)
	}
}

func assertArtifactKinds(t *testing.T, artifacts []ArtifactSummary, want []string) {
	t.Helper()
	got := make([]string, len(artifacts))
	for i := range artifacts {
		got[i] = artifacts[i].Kind
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact kinds = %v, want %v", got, want)
	}
}

type fileSnapshot struct {
	Mode    os.FileMode
	Size    int64
	ModTime int64
	SHA256  string
}

func snapshotTree(t *testing.T, root string) map[string]fileSnapshot {
	t.Helper()
	result := make(map[string]fileSnapshot)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot := fileSnapshot{Mode: info.Mode(), Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash := sha256.Sum256(data)
			snapshot.SHA256 = hex.EncodeToString(hash[:])
		}
		result[relative] = snapshot
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
