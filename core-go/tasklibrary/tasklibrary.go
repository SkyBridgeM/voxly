// Package tasklibrary provides a read-only presentation model over task folders.
// TaskManifest files remain the source of truth; this package does not index,
// cache, migrate, watch, or mutate tasks.
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

const ManifestFilename = "task_manifest.json"

const (
	IssueInvalidManifest    = "invalid_manifest"
	IssueUnsupportedSchema  = "unsupported_schema_version"
	IssueUnreadableManifest = "unreadable_manifest"
)

// TaskSummary is the presentation-safe read model for one processed Task.
type TaskSummary struct {
	TaskID           string            `json:"task_id"`
	Title            string            `json:"title"`
	TitleSource      string            `json:"title_source"`
	Status           string            `json:"status"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
	Tags             []string          `json:"tags"`
	OutputMode       string            `json:"output_mode"`
	PrimaryArtifacts []ArtifactSummary `json:"primary_artifacts"`
	HasPartialErrors bool              `json:"has_partial_errors"`
	TaskFolder       string            `json:"task_folder"`
}

// ArtifactSummary contains only information useful for output presentation.
type ArtifactSummary struct {
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Reference string `json:"reference,omitempty"`
	HasError  bool   `json:"has_error"`
	Retryable *bool  `json:"retryable,omitempty"`
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
	tags := make([]string, 0)
	if manifest.UserMetadata != nil {
		tags = append(tags, manifest.UserMetadata.Tags...)
	}
	return TaskSummary{
		TaskID:           manifest.TaskID,
		Title:            title,
		TitleSource:      titleSource,
		Status:           manifest.Status,
		CreatedAt:        manifest.CreatedAt,
		UpdatedAt:        manifest.UpdatedAt,
		Tags:             tags,
		OutputMode:       manifest.OutputMode,
		PrimaryArtifacts: primaryArtifacts(taskDir, manifest),
		HasPartialErrors: manifest.Status == "completed_with_errors",
		TaskFolder:       taskFolder,
	}
}

func taskTitle(manifest *taskmanifest.TaskManifest) (string, string) {
	if manifest.UserMetadata != nil {
		if title := strings.TrimSpace(manifest.UserMetadata.Title); title != "" {
			return title, "user"
		}
	}
	if name := sanitizedFilenameTitle(manifest.Source.OriginalName); name != "" {
		return name, "filename"
	}
	if createdAt, err := time.Parse(time.RFC3339, manifest.CreatedAt); err == nil {
		return "Task " + createdAt.UTC().Format("2006-01-02 15:04"), "fallback"
	}
	return "Task " + manifest.TaskID, "fallback"
}

func sanitizedFilenameTitle(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	return strings.TrimSpace(name)
}

func primaryArtifacts(taskDir string, manifest *taskmanifest.TaskManifest) []ArtifactSummary {
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
