// Package tasklibrary provides a presentation model and tightly scoped
// user-metadata access over task folders. TaskManifest files remain the source
// of truth; this package does not index, cache, migrate, watch, move, or delete
// tasks or artifacts.
package tasklibrary

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

const ManifestFilename = taskmanifest.ManifestFilename

const (
	IssueInvalidManifest    = "invalid_manifest"
	IssueUnsupportedSchema  = "unsupported_schema_version"
	IssueUnreadableManifest = "unreadable_manifest"
)

// TaskSummary is the presentation-safe read model for one processed Task.
type TaskSummary struct {
	TaskID              string            `json:"task_id"`
	Title               string            `json:"title"`
	TitleSource         string            `json:"title_source"`
	Description         string            `json:"description"`
	Status              string            `json:"status"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
	Tags                []string          `json:"tags"`
	OutputMode          string            `json:"output_mode"`
	PrimaryArtifacts    []ArtifactSummary `json:"primary_artifacts"`
	HasPartialErrors    bool              `json:"has_partial_errors"`
	TaskFolder          string            `json:"task_folder"`
	CanonicalTranscript *ArtifactSummary  `json:"canonical_transcript,omitempty"`
	GeneratedContent    []GenerationGroup `json:"generated_content"`
	CanGenerate         bool              `json:"can_generate"`
}

// ArtifactSummary contains only information useful for output presentation.
type ArtifactSummary struct {
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	Reference      string `json:"reference,omitempty"`
	HasError       bool   `json:"has_error"`
	Retryable      *bool  `json:"retryable,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	Runtime        string `json:"runtime,omitempty"`
	RuntimeVersion string `json:"runtime_version,omitempty"`
	ModelVersion   string `json:"model_version,omitempty"`
}

type GenerationGroup struct {
	TemplateID  string              `json:"template_id"`
	DisplayName string              `json:"display_name"`
	Latest      *GenerationSummary  `json:"latest,omitempty"`
	Versions    []GenerationSummary `json:"versions"`
}

type GenerationSummary struct {
	GenerationID        string `json:"generation_id"`
	TemplateRevision    int    `json:"template_revision,omitempty"`
	TemplateDisplayName string `json:"template_display_name,omitempty"`
	Status              string `json:"status"`
	CreatedAt           string `json:"created_at"`
	Reference           string `json:"reference,omitempty"`
	HasError            bool   `json:"has_error"`
	Retryable           *bool  `json:"retryable,omitempty"`
	Legacy              bool   `json:"legacy"`
	Provider            string `json:"provider,omitempty"`
	Model               string `json:"model,omitempty"`
}

// DiscoveryIssue reports one task folder that could not be represented safely.
type DiscoveryIssue struct {
	TaskFolder string `json:"task_folder"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

// DiscoveryResult separates readable Tasks from isolated per-task issues.
type DiscoveryResult struct {
	Tasks  []TaskSummary    `json:"tasks"`
	Issues []DiscoveryIssue `json:"issues"`
}

// Discover scans direct child directories of root for canonical Task manifests.
// Directories without a manifest are ignored. No files are created or modified.
func Discover(root string) (DiscoveryResult, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("read task root: %w", err)
	}

	result := DiscoveryResult{
		Tasks:  make([]TaskSummary, 0),
		Issues: make([]DiscoveryIssue, 0),
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		taskFolder := entry.Name()
		manifestPath := filepath.Join(root, taskFolder, ManifestFilename)
		if _, err := os.Stat(manifestPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			result.Issues = append(result.Issues, issueFor(taskFolder, err))
			continue
		}

		manifest, err := taskmanifest.LoadManifest(manifestPath)
		if err != nil {
			result.Issues = append(result.Issues, issueFor(taskFolder, err))
			continue
		}
		result.Tasks = append(result.Tasks, summarize(filepath.Join(root, taskFolder), taskFolder, manifest))
	}

	sort.Slice(result.Tasks, func(i, j int) bool {
		left, right := result.Tasks[i], result.Tasks[j]
		if left.UpdatedAt != right.UpdatedAt {
			leftTime, leftErr := time.Parse(time.RFC3339, left.UpdatedAt)
			rightTime, rightErr := time.Parse(time.RFC3339, right.UpdatedAt)
			switch {
			case leftErr == nil && rightErr == nil && !leftTime.Equal(rightTime):
				return leftTime.After(rightTime)
			case leftErr == nil && rightErr != nil:
				return true
			case leftErr != nil && rightErr == nil:
				return false
			case leftErr != nil && rightErr != nil:
				return left.UpdatedAt > right.UpdatedAt
			}
		}
		if left.TaskID != right.TaskID {
			return left.TaskID < right.TaskID
		}
		return left.TaskFolder < right.TaskFolder
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return result.Issues[i].TaskFolder < result.Issues[j].TaskFolder
	})
	return result, nil
}

func summarize(taskDir, taskFolder string, manifest *taskmanifest.TaskManifest) TaskSummary {
	title, titleSource := taskTitle(manifest)
	description := ""
	tags := make([]string, 0)
	if manifest.UserMetadata != nil {
		description = manifest.UserMetadata.Description
		tags = append(tags, manifest.UserMetadata.Tags...)
	}
	primary := primaryArtifacts(taskDir, manifest)
	canonical := canonicalTranscriptSummary(taskDir, manifest)
	groups := generationGroups(taskDir, manifest)
	return TaskSummary{
		TaskID:              manifest.TaskID,
		Title:               title,
		TitleSource:         titleSource,
		Description:         description,
		Status:              manifest.Status,
		CreatedAt:           manifest.CreatedAt,
		UpdatedAt:           manifest.UpdatedAt,
		Tags:                tags,
		OutputMode:          manifest.OutputMode,
		PrimaryArtifacts:    primary,
		HasPartialErrors:    manifest.Status == "completed_with_errors",
		TaskFolder:          taskFolder,
		CanonicalTranscript: canonical,
		GeneratedContent:    groups,
		CanGenerate:         canonical != nil && canonical.Status == "completed" && canonical.Reference != "",
	}
}

func canonicalTranscriptSummary(taskDir string, manifest *taskmanifest.TaskManifest) *ArtifactSummary {
	artifact := summarizeArtifact(taskDir, "transcript", manifest.Artifacts.Transcript)
	if manifest.CanonicalTranscript != nil {
		artifact.Status = manifest.CanonicalTranscript.Status
		artifact.Reference = safeRelativeReference(taskDir, manifest.CanonicalTranscript.ArtifactReference)
		artifact.Provider = manifest.CanonicalTranscript.ASRProvider
		artifact.Model = manifest.CanonicalTranscript.ASRModel
	}
	if manifest.ASRRuntime != nil {
		artifact.Runtime = manifest.ASRRuntime.EngineRuntime
		artifact.RuntimeVersion = manifest.ASRRuntime.RuntimeVersion
		artifact.ModelVersion = manifest.ASRRuntime.ModelVersion
	}
	if artifact.Status == "completed" && (artifact.Reference == "" || !isRegularFile(filepath.Join(taskDir, filepath.FromSlash(artifact.Reference)))) {
		artifact.Status = "unavailable"
		artifact.Reference = ""
	}
	return &artifact
}

func generationGroups(taskDir string, manifest *taskmanifest.TaskManifest) []GenerationGroup {
	groups := make(map[string]*GenerationGroup)
	for _, record := range manifest.Generations {
		group := groups[record.TemplateID]
		if group == nil {
			name := strings.TrimSpace(record.TemplateDisplayName)
			if name == "" {
				name = record.TemplateID
			}
			group = &GenerationGroup{TemplateID: record.TemplateID, DisplayName: name, Versions: make([]GenerationSummary, 0)}
			groups[record.TemplateID] = group
		}
		var retryable *bool
		if record.Error != nil && record.Error.Retryable != nil {
			value := *record.Error.Retryable
			retryable = &value
		}
		status := record.Status
		reference := safeRelativeReference(taskDir, record.ArtifactReference)
		if status == "completed" && (reference == "" || !isRegularFile(filepath.Join(taskDir, filepath.FromSlash(reference)))) {
			status = "unavailable"
			reference = ""
		}
		group.Versions = append(group.Versions, GenerationSummary{GenerationID: record.GenerationID, TemplateRevision: record.TemplateRevision, TemplateDisplayName: record.TemplateDisplayName, Status: status, CreatedAt: record.CreatedAt, Reference: reference, HasError: record.Error != nil, Retryable: retryable, Provider: record.LLMProvider, Model: record.LLMModel})
	}
	appendLegacy := func(id, name string, artifact taskmanifest.Artifact) {
		if artifact.State == "unavailable" && artifact.Path == "" {
			return
		}
		group := groups[id]
		if group == nil {
			group = &GenerationGroup{TemplateID: id, DisplayName: name, Versions: make([]GenerationSummary, 0)}
			groups[id] = group
		}
		summary := summarizeArtifact(taskDir, id, artifact)
		group.Versions = append(group.Versions, GenerationSummary{GenerationID: "legacy-" + id, Status: summary.Status, Reference: summary.Reference, HasError: summary.HasError, Retryable: summary.Retryable, Legacy: true})
	}
	if manifest.OutputMode == "notes" || manifest.OutputMode == "minutes" {
		appendLegacy("legacy-structured-notes", "Legacy notes", manifest.Artifacts.NotesInput)
		if manifest.OutputMode == "minutes" {
			appendLegacy("legacy-meeting-minutes", "Legacy meeting minutes", manifest.Artifacts.Minutes)
		}
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]GenerationGroup, 0, len(ids))
	for _, id := range ids {
		group := groups[id]
		sort.SliceStable(group.Versions, func(i, j int) bool {
			if group.Versions[i].CreatedAt != group.Versions[j].CreatedAt {
				return group.Versions[i].CreatedAt > group.Versions[j].CreatedAt
			}
			return group.Versions[i].GenerationID > group.Versions[j].GenerationID
		})
		for i := range group.Versions {
			if group.Versions[i].Status == "completed" {
				latest := group.Versions[i]
				group.Latest = &latest
				break
			}
		}
		out = append(out, *group)
	}
	return out
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func taskTitle(manifest *taskmanifest.TaskManifest) (string, string) {
	if manifest.UserMetadata != nil {
		if title := strings.TrimSpace(manifest.UserMetadata.Title); title != "" {
			return title, "user"
		}
	}
	if createdAt, err := time.Parse(time.RFC3339, manifest.CreatedAt); err == nil {
		return "Task " + createdAt.UTC().Format("2006-01-02 15:04"), "fallback"
	}
	return "Task " + manifest.TaskID, "fallback"
}

func primaryArtifacts(taskDir string, manifest *taskmanifest.TaskManifest) []ArtifactSummary {
	if manifest.SchemaVersion == taskmanifest.LatestSchemaVersion {
		return []ArtifactSummary{*canonicalTranscriptSummary(taskDir, manifest)}
	}
	artifacts := make([]ArtifactSummary, 0, 3)
	if manifest.OutputMode == "minutes" {
		artifacts = append(artifacts, summarizeArtifact(taskDir, "minutes", manifest.Artifacts.Minutes))
	}
	if manifest.OutputMode == "notes" || manifest.OutputMode == "minutes" {
		artifacts = append(artifacts, summarizeArtifact(taskDir, "notes", manifest.Artifacts.NotesInput))
	}
	artifacts = append(artifacts, summarizeArtifact(taskDir, "transcript", manifest.Artifacts.Transcript))
	return artifacts
}

func summarizeArtifact(taskDir, kind string, artifact taskmanifest.Artifact) ArtifactSummary {
	var retryable *bool
	if artifact.Error != nil && artifact.Error.Retryable != nil {
		value := *artifact.Error.Retryable
		retryable = &value
	}
	return ArtifactSummary{
		Kind:      kind,
		Status:    artifact.State,
		Reference: safeRelativeReference(taskDir, artifact.Path),
		HasError:  artifact.Error != nil,
		Retryable: retryable,
	}
}

func safeRelativeReference(taskDir, artifactPath string) string {
	if strings.TrimSpace(artifactPath) == "" {
		return ""
	}
	absTaskDir, err := filepath.Abs(taskDir)
	if err != nil {
		return ""
	}
	path := artifactPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(absTaskDir, path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	relative, err := filepath.Rel(absTaskDir, absPath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(relative)
}

func issueFor(taskFolder string, err error) DiscoveryIssue {
	switch {
	case errors.Is(err, taskmanifest.ErrUnsupportedSchemaVersion):
		return DiscoveryIssue{TaskFolder: taskFolder, Code: IssueUnsupportedSchema, Message: "Task manifest schema version is unsupported."}
	case errors.Is(err, os.ErrPermission):
		return DiscoveryIssue{TaskFolder: taskFolder, Code: IssueUnreadableManifest, Message: "Task manifest cannot be read."}
	default:
		return DiscoveryIssue{TaskFolder: taskFolder, Code: IssueInvalidManifest, Message: "Task manifest is invalid."}
	}
}
