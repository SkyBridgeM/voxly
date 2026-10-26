package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

const transcriptHashContractVersion = 1

// Generation completion contract. Only an explicit normal stop is recorded as a
// complete result; see classifyGenerationCompletion for the other classes.
const (
	generationCompletionComplete   = "complete"
	generationCompletionIncomplete = "incomplete"
	generationCompletionRefused    = "refused"
	generationCompletionUnverified = "unverified"
	generationCompletionEmpty      = "empty"
)

// maxGenerationInputRunes bounds one generation request. Longer input is
// rejected explicitly instead of being truncated or partially sent; chunked
// summarisation is deliberately out of scope for this work package.
const maxGenerationInputRunes = 120000

// generationOutcome is a provider result: the generated text plus the structured
// reason the provider reported for stopping.
type generationOutcome struct {
	Text         string
	FinishReason string
}

type ContentGenerator interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string, settings *Settings) (string, error)
}

// DetailedContentGenerator reports the provider's structured completion reason
// together with the generated text. A generator that cannot report one is never
// recorded as a verified complete result.
type DetailedContentGenerator interface {
	GenerateDetailed(ctx context.Context, systemPrompt, userPrompt string, settings *Settings) (generationOutcome, error)
}

type deepSeekGenerator struct{}

func (deepSeekGenerator) Generate(ctx context.Context, systemPrompt, userPrompt string, settings *Settings) (string, error) {
	return generateDeepSeekContentContext(ctx, systemPrompt, userPrompt, settings)
}

func (deepSeekGenerator) GenerateDetailed(ctx context.Context, systemPrompt, userPrompt string, settings *Settings) (generationOutcome, error) {
	return generateDeepSeekContentDetailed(ctx, systemPrompt, userPrompt, settings)
}

// generateContentOutcome prefers the structured provider contract and falls back
// to plain text, which carries no completion signal and is therefore classified
// as unverified rather than complete.
func generateContentOutcome(ctx context.Context, generator ContentGenerator, systemPrompt, userPrompt string, settings *Settings) (generationOutcome, error) {
	if detailed, ok := generator.(DetailedContentGenerator); ok {
		return detailed.GenerateDetailed(ctx, systemPrompt, userPrompt, settings)
	}
	text, err := generator.Generate(ctx, systemPrompt, userPrompt, settings)
	if err != nil {
		return generationOutcome{}, err
	}
	return generationOutcome{Text: text}, nil
}

// classifyGenerationCompletion maps a provider finish reason onto the
// completeness contract:
//   - stop              -> complete
//   - length/max_tokens -> incomplete, the output was cut off at a limit
//   - content_filter    -> refused, the provider declined the request
//   - anything else     -> unverified, no trustworthy completion signal
func classifyGenerationCompletion(finishReason string) string {
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "stop":
		return generationCompletionComplete
	case "length", "max_tokens":
		return generationCompletionIncomplete
	case "content_filter":
		return generationCompletionRefused
	default:
		return generationCompletionUnverified
	}
}

// generationCompletionError returns the safe, displayable error for a
// non-complete provider outcome.
func generationCompletionError(completion string) *taskmanifest.ArtifactError {
	switch completion {
	case generationCompletionIncomplete:
		return safeGenerationError("output_incomplete", "The content provider stopped at its output limit, so this generation is incomplete.", true)
	case generationCompletionRefused:
		return safeGenerationError("output_refused", "The content provider declined to generate content for this input.", false)
	case generationCompletionEmpty:
		return safeGenerationError("output_empty", "The content provider returned no text for this request.", true)
	default:
		return safeGenerationError("output_unverified", "The content provider did not report a completion reason, so this output is not verified as complete.", true)
	}
}

// resolveGenerationCompletion classifies a provider outcome and refuses to call
// a normal stop with no text a complete result.
func resolveGenerationCompletion(outcome generationOutcome) string {
	if strings.TrimSpace(outcome.Text) == "" && classifyGenerationCompletion(outcome.FinishReason) == generationCompletionComplete {
		return generationCompletionEmpty
	}
	return classifyGenerationCompletion(outcome.FinishReason)
}

// safeGenerationError builds a displayable generation error that carries no
// provider response body.
func safeGenerationError(code, message string, retryable bool) *taskmanifest.ArtifactError {
	return &taskmanifest.ArtifactError{Code: code, Message: message, Retryable: boolPtr(retryable)}
}

func boolPtr(value bool) *bool {
	return &value
}

// generationInputSizeError rejects input beyond the supported single-request
// boundary instead of sending a request that cannot be answered completely.
func generationInputSizeError(composed ComposedPrompt) error {
	size := utf8.RuneCountInString(composed.User)
	if size <= maxGenerationInputRunes {
		return nil
	}
	return fmt.Errorf("transcript is too large for a single generation request (%d characters, limit %d)", size, maxGenerationInputRunes)
}

// generationExecutionSettings returns the settings one generation attempt must
// run with. A retry reuses the provider and model recorded on the generation
// record, so a later configuration change cannot silently re-bind historical
// provenance to a different model.
func generationExecutionSettings(record *taskmanifest.GenerationRecord, settings *Settings) (*Settings, error) {
	provider := strings.TrimSpace(record.LLMProvider)
	model := strings.TrimSpace(record.LLMModel)
	if provider == "" || model == "" {
		return nil, fmt.Errorf("generation has no recorded source provider")
	}
	effective := *settings
	effective.LLMProvider, effective.DeepSeekModel = provider, model
	if err := ValidateLLMProviderConfig(effective.LLMConfig()); err != nil {
		return nil, err
	}
	return &effective, nil
}

// generationRetryable reports whether one generation record may run again under
// its own identity: it either never finished, or failed in a way that a retry
// can plausibly change.
func generationRetryable(record *taskmanifest.GenerationRecord) bool {
	if record.Status == "processing" {
		return true
	}
	return record.Status == "failed" && record.Error != nil && record.Error.Retryable != nil && *record.Error.Retryable
}

// pendingGenerationIndex returns the generation record a resume would retry, or
// -1 when the last generation is not retryable.
func pendingGenerationIndex(m *Manifest) int {
	if len(m.Generations) == 0 {
		return -1
	}
	index := len(m.Generations) - 1
	if !generationRetryable(&m.Generations[index]) {
		return -1
	}
	return index
}

// latestCompletedGenerationReference returns the artifact reference of the most
// recent completed generation, or "" when none finished.
func latestCompletedGenerationReference(m *Manifest) string {
	for i := len(m.Generations) - 1; i >= 0; i-- {
		if m.Generations[i].Status == "completed" && m.Generations[i].ArtifactReference != "" {
			return m.Generations[i].ArtifactReference
		}
	}
	return ""
}

// generationIndex returns the index of one generation record, or -1.
func generationIndex(m *Manifest, generationID string) int {
	for i := range m.Generations {
		if m.Generations[i].GenerationID == generationID {
			return i
		}
	}
	return -1
}

// resumeGenerationSettings returns the settings the content-generation stage
// must execute with: the retried generation's recorded identity when one is
// pending, and the caller's configuration otherwise.
func resumeGenerationSettings(m *Manifest, settings *Settings) (*Settings, error) {
	index := pendingGenerationIndex(m)
	if index < 0 {
		return settings, nil
	}
	return generationExecutionSettings(&m.Generations[index], settings)
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

func validateTemplateArtifactID(id string) error {
	if _, ok := builtInTemplates[id]; ok {
		return nil
	}
	return validateCustomTemplateID(id)
}

func validatedGeneratedArtifactPath(taskDir, templateID, generationID, extension string) (string, string, error) {
	if err := validateTemplateArtifactID(templateID); err != nil {
		return "", "", err
	}
	if extension != ".md" || generationID == "" {
		return "", "", fmt.Errorf("generated artifact identity is invalid")
	}
	for _, char := range generationID {
		if char != '_' && char != '-' && (char < '0' || char > '9') && (char < 'a' || char > 'z') {
			return "", "", fmt.Errorf("generated artifact identity is invalid")
		}
	}
	root, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve task folder: %w", err)
	}
	reference := filepath.Join("generated", templateID, generationID+extension)
	target := filepath.Join(root, reference)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("artifact path is outside task folder")
	}
	existing := filepath.Dir(target)
	for {
		_, statErr := os.Lstat(existing)
		if statErr == nil {
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", "", fmt.Errorf("generated artifact path is unsafe")
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", "", fmt.Errorf("generated artifact path is unsafe")
		}
		existing = parent
	}
	resolvedAncestor, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", "", fmt.Errorf("generated artifact path is unsafe")
	}
	ancestorRel, err := filepath.Rel(root, resolvedAncestor)
	if err != nil || ancestorRel == ".." || filepath.IsAbs(ancestorRel) || strings.HasPrefix(ancestorRel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("generated artifact path is outside task folder")
	}
	return target, filepath.ToSlash(reference), nil
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

// canonicalTranscriptASRIdentity reports the speech-recognition identity behind
// the merged transcript. A Task-level identity is authoritative; otherwise a
// single identity shared by every completed chunk is used. A completed chunk
// with unrecorded provenance, or chunks that disagree, leave the identity
// unknown instead of guessing one.
func canonicalTranscriptASRIdentity(m *Manifest) (string, string) {
	if provider, model := strings.TrimSpace(m.Chunks.ASRProvider), strings.TrimSpace(m.Chunks.ASRModel); provider != "" && model != "" {
		return provider, model
	}
	provider, model, seen := "", "", false
	for i := range m.Chunks.Items {
		item := &m.Chunks.Items[i]
		if item.State != ChunkCompleted {
			continue
		}
		itemProvider, itemModel := strings.TrimSpace(item.ASRProvider), strings.TrimSpace(item.ASRModel)
		if itemProvider == "" || itemModel == "" {
			return "", ""
		}
		if !seen {
			provider, model, seen = itemProvider, itemModel, true
			continue
		}
		if provider != itemProvider || model != itemModel {
			return "", ""
		}
	}
	return provider, model
}

func initializeCanonicalTranscript(taskDir string, m *Manifest) error {
	revisionID, err := randomID("tr")
	if err != nil {
		return err
	}
	asrProvider, asrModel := canonicalTranscriptASRIdentity(m)
	m.CanonicalTranscript = &taskmanifest.CanonicalTranscript{TranscriptID: "canonical", RevisionID: revisionID, CreatedAt: nowFunc().UTC().Format(time.RFC3339), ASRProvider: asrProvider, ASRModel: asrModel, Status: "pending"}
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
	return GenerateContentContext(context.Background(), taskDir, m, templateID, settings, generator)
}

// GenerateContentContext is the cancellable form used by the CLI entry points.
func GenerateContentContext(ctx context.Context, taskDir string, m *Manifest, templateID string, settings *Settings, generator ContentGenerator) (*taskmanifest.GenerationRecord, error) {
	if err := ValidateLLMProviderConfig(settings.LLMConfig()); err != nil {
		return nil, err
	}
	template, err := LookupPromptTemplateWithStore(settings.TemplateStorePath, templateID)
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
	composed := ComposePrompt(template, string(data))
	if err := generationInputSizeError(composed); err != nil {
		return nil, err
	}
	generationID, err := randomID("gen")
	if err != nil {
		return nil, err
	}
	now := nowFunc().UTC().Format(time.RFC3339)
	record := taskmanifest.GenerationRecord{GenerationID: generationID, TemplateID: template.ID, TemplateRevision: template.Revision, TemplateDisplayName: template.DisplayName["en"], TemplateInstructions: strings.TrimSpace(template.Instructions), CreatedAt: now, UpdatedAt: now, SourceTranscriptRevisionID: m.CanonicalTranscript.RevisionID, SourceTranscriptSHA256: m.CanonicalTranscript.ContentSHA256, LLMProvider: settings.LLMProvider, LLMModel: settings.DeepSeekModel, PromptCompositionVersion: promptCompositionVersion, PromptHash: composed.Hash, Status: "processing", Attempts: []taskmanifest.GenerationAttempt{{Attempt: 1, CreatedAt: now, StartedAt: now, Status: "processing", LLMProvider: settings.LLMProvider, LLMModel: settings.DeepSeekModel}}}
	m.Generations = append(m.Generations, record)
	index := len(m.Generations) - 1
	if err := SaveManifest(taskDir, m); err != nil {
		return nil, err
	}
	if err := executeGenerationAttempt(ctx, taskDir, m, index, template.ID, template.Extension, settings, generator, composed); err != nil {
		return &m.Generations[index], err
	}
	return &m.Generations[index], nil
}

// executeGenerationAttempt runs the provider call for an already persisted
// generation record and stores the classified outcome. A fresh generation and
// an identity-bound retry share it so provider dispatch, completion
// classification, artifact writing and failure handling cannot drift apart.
func executeGenerationAttempt(ctx context.Context, taskDir string, m *Manifest, index int, templateID, extension string, settings *Settings, generator ContentGenerator, composed ComposedPrompt) error {
	record := &m.Generations[index]
	path, reference, err := validatedGeneratedArtifactPath(taskDir, templateID, record.GenerationID, extension)
	if err != nil {
		failGenerationRecord(taskDir, m, index, safeGenerationError("artifact_write_failed", "Generated artifact path is unsafe.", false))
		return err
	}
	outcome, err := generateContentOutcome(ctx, generator, composed.System, composed.User, settings)
	if err != nil {
		if cancellationRequested(ctx, err) {
			// A requested stop is not a provider failure. The version keeps its
			// identity and stays retryable, and the attempt records the cancel so
			// the desktop can tell the two apart.
			failGenerationRecord(taskDir, m, index, safeGenerationError(cancellationErrorCode, "The generation was cancelled before it finished.", true))
			return cancelledError()
		}
		failGenerationRecord(taskDir, m, index, safeGenerationError("generation_failed", "Content generation provider request failed.", IsRetryableError(err)))
		return fmt.Errorf("content generation provider request failed")
	}
	completion := resolveGenerationCompletion(outcome)
	recordAttemptCompletion(record, completion, outcome.FinishReason)
	if completion == generationCompletionComplete {
		if err := writeGeneratedArtifact(path, outcome.Text); err != nil {
			failGenerationRecord(taskDir, m, index, safeGenerationError("artifact_write_failed", "Generated artifact could not be written.", false))
			return err
		}
		record.Status, record.Error, record.ArtifactReference, record.UpdatedAt = "completed", nil, reference, nowFunc().UTC().Format(time.RFC3339)
		record.Attempts[len(record.Attempts)-1].Status = "completed"
		return SaveManifest(taskDir, m)
	}
	// Keep whatever usable text arrived, but never present a non-complete
	// outcome as a normal result.
	if strings.TrimSpace(outcome.Text) != "" {
		if writeErr := writeGeneratedArtifact(path, outcome.Text); writeErr == nil {
			record.ArtifactReference = reference
		}
	}
	if err := failGenerationAttempt(taskDir, m, index, generationCompletionError(completion)); err != nil {
		return err
	}
	return fmt.Errorf("content generation did not complete")
}

// recordAttemptCompletion stores the structured provider result on the attempt
// that just ran.
func recordAttemptCompletion(record *taskmanifest.GenerationRecord, completion, finishReason string) {
	last := len(record.Attempts) - 1
	if last < 0 {
		return
	}
	record.Attempts[last].Completion = completion
	record.Attempts[last].FinishReason = finishReason
}

func writeGeneratedArtifact(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create generated artifact directory: %w", err)
	}
	return WriteFileAtomically(path, []byte(strings.TrimSpace(text)+"\n"), 0600)
}

func failGenerationRecord(taskDir string, m *Manifest, index int, safeErr *taskmanifest.ArtifactError) {
	_ = failGenerationAttempt(taskDir, m, index, safeErr)
}

// failGenerationAttempt marks the generation and its current attempt failed and
// persists the safe error.
func failGenerationAttempt(taskDir string, m *Manifest, index int, safeErr *taskmanifest.ArtifactError) error {
	m.Generations[index].Status = "failed"
	m.Generations[index].Error = safeErr
	m.Generations[index].UpdatedAt = nowFunc().UTC().Format(time.RFC3339)
	last := len(m.Generations[index].Attempts) - 1
	if last >= 0 {
		m.Generations[index].Attempts[last].Status = "failed"
		m.Generations[index].Attempts[last].Error = safeErr
	}
	return SaveManifest(taskDir, m)
}

// ResumeOrGenerateContent retries an interrupted/retryable generation under
// the same generation ID. A fresh user request still uses GenerateContent and
// therefore receives a new ID.
func ResumeOrGenerateContent(taskDir string, m *Manifest, settings *Settings, generator ContentGenerator) error {
	return ResumeOrGenerateContentContext(context.Background(), taskDir, m, settings, generator)
}

// ResumeOrGenerateContentContext continues whatever the Task still owes: it
// retries the pending generation, or creates the first one. Retrying a specific
// record the user picked is RetryGeneration, not this function.
func ResumeOrGenerateContentContext(ctx context.Context, taskDir string, m *Manifest, settings *Settings, generator ContentGenerator) error {
	if err := ensureRetryableCanonicalTranscript(taskDir, m); err != nil {
		return err
	}
	if len(m.Generations) == 0 {
		_, err := GenerateContentContext(ctx, taskDir, m, m.RequestedTemplateID, settings, generator)
		return err
	}
	index := pendingGenerationIndex(m)
	if index < 0 {
		return nil
	}
	return retryGenerationRecord(ctx, taskDir, m, index, settings, generator)
}

// RetryGeneration retries exactly one generation record chosen by the caller.
// The record keeps its identity: same generation ID, same provider and model,
// same template snapshot and source transcript, with a new attempt appended.
func RetryGeneration(ctx context.Context, taskDir string, m *Manifest, generationID string, settings *Settings, generator ContentGenerator) error {
	if strings.TrimSpace(generationID) == "" {
		return fmt.Errorf("a generation id is required to retry")
	}
	index := generationIndex(m, generationID)
	if index < 0 {
		return fmt.Errorf("generation %s was not found in this task", generationID)
	}
	if !generationRetryable(&m.Generations[index]) {
		return fmt.Errorf("generation %s is not retryable", generationID)
	}
	if err := ensureRetryableCanonicalTranscript(taskDir, m); err != nil {
		return err
	}
	return retryGenerationRecord(ctx, taskDir, m, index, settings, generator)
}

// ensureRetryableCanonicalTranscript makes sure a canonical transcript exists
// before a generation attempt is prepared.
func ensureRetryableCanonicalTranscript(taskDir string, m *Manifest) error {
	if m.CanonicalTranscript != nil && m.CanonicalTranscript.Status == "completed" {
		return nil
	}
	if err := ensureCanonicalTranscript(taskDir, m); err != nil {
		return err
	}
	return SaveManifest(taskDir, m)
}

// retryGenerationRecord re-binds one existing record to its recorded identity
// and executes a new attempt. Both the automatic resume and the explicit retry
// go through it, so a retry can never diverge from the recorded provenance.
func retryGenerationRecord(ctx context.Context, taskDir string, m *Manifest, index int, settings *Settings, generator ContentGenerator) error {
	record := &m.Generations[index]
	if m.CanonicalTranscript == nil || m.CanonicalTranscript.Status != "completed" {
		return fmt.Errorf("canonical transcript is not completed")
	}
	// A retry keeps the recorded provider, model, template snapshot and source
	// transcript; it never re-binds to whatever is configured now.
	effective, err := generationExecutionSettings(record, settings)
	if err != nil {
		return err
	}
	if record.SourceTranscriptRevisionID != m.CanonicalTranscript.RevisionID {
		return fmt.Errorf("generation source transcript revision has changed — start a new generation")
	}
	template := PromptTemplate{ID: record.TemplateID, Revision: record.TemplateRevision, DisplayName: map[string]string{"en": record.TemplateDisplayName, "zh": record.TemplateDisplayName}, Instructions: record.TemplateInstructions, ContentType: "text/markdown", Extension: ".md"}
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
	if err := generationInputSizeError(composed); err != nil {
		return err
	}
	now := nowFunc().UTC().Format(time.RFC3339)
	record.Status, record.Error, record.UpdatedAt = "processing", nil, now
	record.Attempts = append(record.Attempts, taskmanifest.GenerationAttempt{Attempt: len(record.Attempts) + 1, CreatedAt: now, StartedAt: now, Status: "processing", LLMProvider: effective.LLMProvider, LLMModel: effective.DeepSeekModel})
	if err := SaveManifest(taskDir, m); err != nil {
		return err
	}
	return executeGenerationAttempt(ctx, taskDir, m, index, template.ID, template.Extension, effective, generator, composed)
}
