package tasklibrary

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

const (
	MaxTitleLength       = 200
	MaxDescriptionLength = 4000
	MaxTags              = 20
	MaxTagLength         = 50
)

var (
	ErrInvalidTaskID = errors.New("invalid task ID")
	ErrTaskNotFound  = errors.New("task not found")
	safeTaskID       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
)

// TaskMetadata is the user-managed metadata view for one Task.
type TaskMetadata struct {
	TaskID      string   `json:"task_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// MetadataUpdate contains only user-owned fields. Execution and artifact
// state are always retained from the existing TaskManifest.
type MetadataUpdate struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// ReadMetadata rediscovers a Task under root and returns its user metadata.
func ReadMetadata(root, taskID string) (TaskMetadata, error) {
	_, manifest, err := findTaskManifest(root, taskID)
	if err != nil {
		return TaskMetadata{}, err
	}
	return metadataView(manifest), nil
}

// UpdateMetadata rediscovers a Task under root and atomically updates only
// its user-managed fields. Task folders and artifacts are never moved.
func UpdateMetadata(root, taskID string, update MetadataUpdate) (TaskMetadata, error) {
	return updateMetadataAt(root, taskID, update, time.Now().UTC())
}

func updateMetadataAt(root, taskID string, update MetadataUpdate, now time.Time) (TaskMetadata, error) {
	manifestPath, manifest, err := findTaskManifest(root, taskID)
	if err != nil {
		return TaskMetadata{}, err
	}
	normalized, err := normalizeMetadataUpdate(update)
	if err != nil {
		return TaskMetadata{}, err
	}
	if manifest.UserMetadata == nil {
		manifest.UserMetadata = &taskmanifest.UserMetadata{}
	}
	manifest.UserMetadata.Title = normalized.Title
	manifest.UserMetadata.Description = normalized.Description
	manifest.UserMetadata.Tags = append([]string(nil), normalized.Tags...)
	manifest.UpdatedAt = now.Format(time.RFC3339)
	if err := taskmanifest.WriteManifestAtomic(manifestPath, manifest); err != nil {
		return TaskMetadata{}, fmt.Errorf("update task metadata: %w", err)
	}
	return metadataView(manifest), nil
}

func normalizeMetadataUpdate(update MetadataUpdate) (MetadataUpdate, error) {
	update.Title = strings.TrimSpace(update.Title)
	update.Description = strings.TrimSpace(update.Description)
	if len([]rune(update.Title)) > MaxTitleLength {
		return MetadataUpdate{}, fmt.Errorf("title exceeds %d characters", MaxTitleLength)
	}
	if len([]rune(update.Description)) > MaxDescriptionLength {
		return MetadataUpdate{}, fmt.Errorf("description exceeds %d characters", MaxDescriptionLength)
	}
	if len(update.Tags) > MaxTags {
		return MetadataUpdate{}, fmt.Errorf("tags exceed maximum of %d", MaxTags)
	}
	seen := make(map[string]struct{}, len(update.Tags))
	tags := make([]string, 0, len(update.Tags))
	for _, raw := range update.Tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if len([]rune(tag)) > MaxTagLength {
			return MetadataUpdate{}, fmt.Errorf("tag exceeds %d characters", MaxTagLength)
		}
		if strings.ContainsAny(tag, "\r\n\t") {
			return MetadataUpdate{}, errors.New("tag contains unsupported whitespace")
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	update.Tags = tags
	return update, nil
}

func metadataView(manifest *taskmanifest.TaskManifest) TaskMetadata {
	title, _ := taskTitle(manifest)
	metadata := TaskMetadata{
		TaskID: manifest.TaskID, Title: title,
		CreatedAt: manifest.CreatedAt, UpdatedAt: manifest.UpdatedAt,
		Tags: make([]string, 0),
	}
	if manifest.UserMetadata != nil {
		metadata.Description = manifest.UserMetadata.Description
		metadata.Tags = append(metadata.Tags, manifest.UserMetadata.Tags...)
	}
	return metadata
}

func findTaskManifest(root, taskID string) (string, *taskmanifest.TaskManifest, error) {
	taskID = strings.TrimSpace(taskID)
	if !safeTaskID.MatchString(taskID) {
		return "", nil, ErrInvalidTaskID
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", nil, fmt.Errorf("resolve task root: %w", err)
	}
	canonicalRoot, err = filepath.Abs(canonicalRoot)
	if err != nil {
		return "", nil, fmt.Errorf("resolve task root: %w", err)
	}
	rootInfo, err := os.Stat(canonicalRoot)
	if err != nil || !rootInfo.IsDir() {
		return "", nil, errors.New("task root is unavailable")
	}
	entries, err := os.ReadDir(canonicalRoot)
	if err != nil {
		return "", nil, errors.New("task root is unavailable")
	}
	var foundPath string
	var foundManifest *taskmanifest.TaskManifest
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		taskDir := filepath.Join(canonicalRoot, entry.Name())
		canonicalTask, err := filepath.EvalSymlinks(taskDir)
		if err != nil || filepath.Dir(canonicalTask) != canonicalRoot {
			continue
		}
		manifestPath := filepath.Join(canonicalTask, ManifestFilename)
		info, err := os.Lstat(manifestPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		manifest, err := taskmanifest.LoadManifest(manifestPath)
		if err != nil || manifest.TaskID != taskID {
			continue
		}
		if foundManifest != nil {
			return "", nil, errors.New("task identity is ambiguous")
		}
		foundPath, foundManifest = manifestPath, manifest
	}
	if foundManifest == nil {
		return "", nil, ErrTaskNotFound
	}
	return foundPath, foundManifest, nil
}
