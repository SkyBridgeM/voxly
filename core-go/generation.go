package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

const transcriptHashContractVersion = 1

type ContentGenerator interface {
	Generate(systemPrompt, userPrompt string, settings *Settings) (string, error)
}

type deepSeekGenerator struct{}

func (deepSeekGenerator) Generate(systemPrompt, userPrompt string, settings *Settings) (string, error) {
	return generateDeepSeekContent(systemPrompt, userPrompt, settings)
}

func randomID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

func relativeTaskReference(taskDir, path string) (string, error) {
	root, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		return "", fmt.Errorf("resolve task folder: %w", err)
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve artifact: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path is outside task folder")
	}
	return filepath.ToSlash(rel), nil
}

func validatedTaskFile(taskDir, reference string) (string, error) {
	if filepath.IsAbs(reference) || strings.Contains(reference, "..") {
		return "", fmt.Errorf("artifact reference is invalid")
	}
	root, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		return "", fmt.Errorf("resolve task folder: %w", err)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(reference)))
	if err != nil {
		return "", fmt.Errorf("resolve task artifact: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact is outside task folder")
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("artifact is not a regular file")
	}
	return target, nil
}

func initializeCanonicalTranscript(taskDir string, m *Manifest) error {
	revisionID, err := randomID("tr")
	if err != nil {
		return err
	}
	m.CanonicalTranscript = &taskmanifest.CanonicalTranscript{TranscriptID: "canonical", RevisionID: revisionID, CreatedAt: nowFunc().UTC().Format(time.RFC3339), ASRProvider: m.Chunks.ASRProvider, ASRModel: m.Chunks.ASRModel, Status: "pending"}
	return nil
}

func completeCanonicalTranscript(taskDir, transcriptPath string, m *Manifest) error {
	if m.CanonicalTranscript == nil {
		if err := initializeCanonicalTranscript(taskDir, m); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return fmt.Errorf("read canonical transcript: %w", err)
	}
	sum := sha256.Sum256(data)
	reference, err := relativeTaskReference(taskDir, transcriptPath)
	if err != nil {
		return err
	}
	m.CanonicalTranscript.ArtifactReference = reference
	m.CanonicalTranscript.ContentSHA256 = hex.EncodeToString(sum[:])
	m.CanonicalTranscript.HashContractVersion = transcriptHashContractVersion
	m.CanonicalTranscript.Status = "completed"
	return nil
}

func ensureCanonicalTranscript(taskDir string, m *Manifest) error {
	if m.CanonicalTranscript != nil && m.CanonicalTranscript.Status == "completed" {
		return nil
	}
	path, err := validatedTaskFile(taskDir, "transcript.md")
	if err != nil {
		return fmt.Errorf("canonical transcript is missing or unsafe")
	}
	if err := initializeCanonicalTranscript(taskDir, m); err != nil {
		return err
	}
	if err := completeCanonicalTranscript(taskDir, path, m); err != nil {
		return err
	}
	m.SchemaVersion = taskmanifest.LatestSchemaVersion
	if m.OutputMode == "transcript" {
		m.OutputMode = "transcript_only"
	}
	return nil
}

// GenerateContent appends one user-requested generation. Callers operating on
// an existing Task must hold that Task's filesystem lock.
func GenerateContent(taskDir string, m *Manifest, templateID string, settings *Settings, generator ContentGenerator) (*taskmanifest.GenerationRecord, error) {
	if err := ValidateLLMProviderConfig(settings.LLMConfig()); err != nil {
		return nil, err
	}
	template, err := LookupPromptTemplate(templateID)
	if err != nil {
		return nil, err
	}
	if m.CanonicalTranscript == nil || m.CanonicalTranscript.Status != "completed" {
		return nil, fmt.Errorf("canonical transcript is not completed")
	}
	transcriptPath, err := validatedTaskFile(taskDir, m.CanonicalTranscript.ArtifactReference)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return nil, fmt.Errorf("read canonical transcript: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != m.CanonicalTranscript.ContentSHA256 {
		return nil, fmt.Errorf("canonical transcript integrity mismatch")
	}
	generationID, err := randomID("gen")
	if err != nil {
		return nil, err
	}
	now := nowFunc().UTC().Format(time.RFC3339)
	composed := ComposePrompt(template, string(data))
	record := taskmanifest.GenerationRecord{GenerationID: generationID, TemplateID: template.ID, TemplateRevision: template.Revision, TemplateDisplayName: template.DisplayName["en"], TemplateInstructions: strings.TrimSpace(template.Instructions), CreatedAt: now, UpdatedAt: now, SourceTranscriptRevisionID: m.CanonicalTranscript.RevisionID, SourceTranscriptSHA256: m.CanonicalTranscript.ContentSHA256, LLMProvider: settings.LLMProvider, LLMModel: settings.DeepSeekModel, PromptCompositionVersion: promptCompositionVersion, PromptHash: composed.Hash, Status: "processing", Attempts: []taskmanifest.GenerationAttempt{{Attempt: 1, CreatedAt: now, Status: "processing"}}}
	m.Generations = append(m.Generations, record)
	index := len(m.Generations) - 1
	if err := SaveManifest(taskDir, m); err != nil {
		return nil, err
	}
	text, generationErr := generator.Generate(composed.System, composed.User, settings)
	if generationErr != nil {
		retryable := IsRetryableError(generationErr)
		failGenerationRecord(taskDir, m, index, "generation_failed", "Content generation provider request failed.", retryable)
		return &m.Generations[index], fmt.Errorf("content generation provider request failed")
	}
	path := filepath.Join(taskDir, "generated", template.ID, generationID+template.Extension)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		failGenerationRecord(taskDir, m, index, "artifact_write_failed", "Generated artifact could not be written.", false)
		return &m.Generations[index], fmt.Errorf("create generated artifact directory: %w", err)
	}
	if err := WriteFileAtomically(path, []byte(strings.TrimSpace(text)+"\n"), 0600); err != nil {
		failGenerationRecord(taskDir, m, index, "artifact_write_failed", "Generated artifact could not be written.", false)
		return &m.Generations[index], err
	}
	reference, err := relativeTaskReference(taskDir, path)
	if err != nil {
		return nil, err
	}
	m.Generations[index].Status = "completed"
	m.Generations[index].ArtifactReference = reference
	m.Generations[index].UpdatedAt = nowFunc().UTC().Format(time.RFC3339)
	m.Generations[index].Attempts[0].Status = "completed"
	if err := SaveManifest(taskDir, m); err != nil {
		return nil, err
	}
	return &m.Generations[index], nil
}

func failGenerationRecord(taskDir string, m *Manifest, index int, code, message string, retryable bool) {
	safeErr := &ArtifactError{Code: code, Message: message, Retryable: &retryable}
	m.Generations[index].Status = "failed"
	m.Generations[index].Error = safeErr
	m.Generations[index].UpdatedAt = nowFunc().UTC().Format(time.RFC3339)
	last := len(m.Generations[index].Attempts) - 1
	if last >= 0 {
		m.Generations[index].Attempts[last].Status = "failed"
		m.Generations[index].Attempts[last].Error = safeErr
	}
	_ = SaveManifest(taskDir, m)
}

// ResumeOrGenerateContent retries an interrupted/retryable generation under
// the same generation ID. A fresh user request still uses GenerateContent and
// therefore receives a new ID.
func ResumeOrGenerateContent(taskDir string, m *Manifest, settings *Settings, generator ContentGenerator) error {
	if m.CanonicalTranscript == nil || m.CanonicalTranscript.Status != "completed" {
		if err := ensureCanonicalTranscript(taskDir, m); err != nil {
			return err
		}
		if err := SaveManifest(taskDir, m); err != nil {
			return err
		}
	}
	if len(m.Generations) == 0 {
		_, err := GenerateContent(taskDir, m, m.RequestedTemplateID, settings, generator)
		return err
	}
	index := len(m.Generations) - 1
	record := &m.Generations[index]
	eligible := record.Status == "processing" || (record.Status == "failed" && record.Error != nil && record.Error.Retryable != nil && *record.Error.Retryable)
	if !eligible {
		return nil
	}
	template, err := LookupPromptTemplate(record.TemplateID)
	if err != nil {
		return err
	}
	template.Revision = record.TemplateRevision
	template.Instructions = record.TemplateInstructions
	if m.CanonicalTranscript == nil || m.CanonicalTranscript.Status != "completed" {
		return fmt.Errorf("canonical transcript is not completed")
	}
	transcriptPath, err := validatedTaskFile(taskDir, m.CanonicalTranscript.ArtifactReference)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return fmt.Errorf("read canonical transcript: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != record.SourceTranscriptSHA256 {
		return fmt.Errorf("canonical transcript integrity mismatch")
	}
	composed := ComposePrompt(template, string(data))
	if composed.Hash != record.PromptHash {
		return fmt.Errorf("generation prompt integrity mismatch")
	}
	now := nowFunc().UTC().Format(time.RFC3339)
	record.Status, record.Error, record.UpdatedAt = "processing", nil, now
	record.Attempts = append(record.Attempts, taskmanifest.GenerationAttempt{Attempt: len(record.Attempts) + 1, CreatedAt: now, Status: "processing"})
	if err := SaveManifest(taskDir, m); err != nil {
		return err
	}
	text, err := generator.Generate(composed.System, composed.User, settings)
	if err != nil {
		failGenerationRecord(taskDir, m, index, "generation_failed", "Content generation provider request failed.", IsRetryableError(err))
		return fmt.Errorf("content generation provider request failed")
	}
	path := filepath.Join(taskDir, "generated", template.ID, record.GenerationID+template.Extension)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		failGenerationRecord(taskDir, m, index, "artifact_write_failed", "Generated artifact could not be written.", false)
		return err
	}
	if err := WriteFileAtomically(path, []byte(strings.TrimSpace(text)+"\n"), 0600); err != nil {
		failGenerationRecord(taskDir, m, index, "artifact_write_failed", "Generated artifact could not be written.", false)
		return err
	}
	reference, err := relativeTaskReference(taskDir, path)
	if err != nil {
		return err
	}
	record.Status, record.Error, record.ArtifactReference, record.UpdatedAt = "completed", nil, reference, nowFunc().UTC().Format(time.RFC3339)
	record.Attempts[len(record.Attempts)-1].Status = "completed"
	return SaveManifest(taskDir, m)
}
