package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProgressFunc is called during transcription with status updates.
type ProgressFunc func(action string, index, total int)

// TranscribeChunks transcribes all chunks, skipping already-completed transcripts.
func TranscribeChunks(chunks []string, transcriptsDir string, settings *Settings, progressFn ProgressFunc) ([]string, error) {
	return TranscribeChunksContext(context.Background(), chunks, transcriptsDir, settings, progressFn)
}

func TranscribeChunksContext(ctx context.Context, chunks []string, transcriptsDir string, settings *Settings, progressFn ProgressFunc) ([]string, error) {
	if err := ValidateASRProviderConfig(settings.ASRConfig()); err != nil {
		return nil, err
	}
	apiKey := ""
	if settings.ASRConfig().ProviderID == ASRProviderMimo {
		var err error
		apiKey, err = settings.RequireMimoAPIKey()
		if err != nil {
			return nil, err
		}
	}
	provider, err := ResolveASRProvider(settings.ASRConfig(), settings, apiKey)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(transcriptsDir, 0700); err != nil {
		return nil, err
	}

	var outputs []string
	total := len(chunks)

	for i, chunk := range chunks {
		index := i + 1
		transcriptPath := filepath.Join(transcriptsDir, strings.TrimSuffix(filepath.Base(chunk), filepath.Ext(chunk))+".txt")

		if isCompleteFile(transcriptPath) {
			if progressFn != nil {
				progressFn("skipped", index, total)
			}
			fmt.Printf("Skipping existing transcript: %s\n", filepath.Base(transcriptPath))
			outputs = append(outputs, transcriptPath)
			continue
		}

		if progressFn != nil {
			progressFn("transcribing", index, total)
		}
		fmt.Printf("Transcribing %s...\n", filepath.Base(chunk))

		result, err := provider.Transcribe(ctx, ASRRequest{AudioPath: chunk, Language: "auto"})
		if err != nil {
			return nil, fmt.Errorf("transcribing %s: %w", filepath.Base(chunk), err)
		}

		if err := os.WriteFile(transcriptPath, []byte(strings.TrimSpace(result.Text)+"\n"), 0600); err != nil {
			return nil, err
		}

		if progressFn != nil {
			progressFn("completed", index, total)
		}
		outputs = append(outputs, transcriptPath)
	}

	return outputs, nil
}

type mimoASRProvider struct {
	settings *Settings
	apiKey   string
}

func (provider mimoASRProvider) Transcribe(ctx context.Context, request ASRRequest) (ASRResult, error) {
	text, err := transcribeAudioFileContext(ctx, request.AudioPath, provider.settings, provider.apiKey)
	return ASRResult{Text: text}, err
}

func transcribeAudioFile(audioPath string, settings *Settings, apiKey string) (string, error) {
	return transcribeAudioFileContext(context.Background(), audioPath, settings, apiKey)
}

func transcribeAudioFileContext(ctx context.Context, audioPath string, settings *Settings, apiKey string) (string, error) {
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(audioPath)), ".")
	if format != "mp3" && format != "wav" {
		return "", fmt.Errorf("MiMo ASR only supports mp3 and wav, got %q (convert to wav first)", format)
	}

	data, err := os.ReadFile(audioPath)
	if err != nil {
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(data)

	payload := BuildMimoASRPayload(b64, format, settings.MimoModel)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 1; attempt <= settings.RetryCount; attempt++ {
		text, err := callMimoAPIContext(ctx, apiKey, settings.MimoEndpoint, body, settings.TimeoutSeconds)
		if err == nil {
			return text, nil
		}
		lastErr = err

		// Don't retry non-retriable client errors (e.g. 401, 403).
		if isHTTPClientError(err) && !isRetriableHTTPError(err) {
			break
		}

		if attempt < settings.RetryCount {
			backoff := retryBackoff(err, attempt)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}
	}

	return "", fmt.Errorf("failed to transcribe %s after %d attempts: %w", filepath.Base(audioPath), settings.RetryCount, lastErr)
}

func callMimoAPI(apiKey, endpoint string, body []byte, timeoutSeconds int) (string, error) {
	return callMimoAPIContext(context.Background(), apiKey, endpoint, body, timeoutSeconds)
}

func callMimoAPIContext(ctx context.Context, apiKey, endpoint string, body []byte, timeoutSeconds int) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("MiMo server error %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}
	if resp.StatusCode >= 400 {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return "", &httpClientError{
			statusCode: resp.StatusCode,
			message:    fmt.Sprintf("MiMo API error %d: %s", resp.StatusCode, truncate(string(respBody), 1000)),
			retryAfter: retryAfter,
		}
	}

	return ExtractTranscript(respBody)
}

// BuildMimoASRPayload constructs the JSON payload for MiMo ASR.
func BuildMimoASRPayload(audioB64, audioFormat, model string) map[string]interface{} {
	mimeTypes := map[string]string{
		"wav": "audio/wav",
		"mp3": "audio/mpeg",
	}
	mime := mimeTypes[audioFormat]
	dataURL := fmt.Sprintf("data:%s;base64,%s", mime, audioB64)

	return map[string]interface{}{
		"model":       model,
		"asr_options": map[string]string{"language": "auto"},
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "input_audio",
						"input_audio": map[string]string{
							"data": dataURL,
						},
					},
				},
			},
		},
	}
}

// ExtractTranscript parses the MiMo API response and extracts the transcript text.
func ExtractTranscript(body []byte) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("invalid JSON response: %w", err)
	}

	choices, ok := data["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("unexpected MiMo response shape: missing choices")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected MiMo response shape: invalid choice")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected MiMo response shape: missing message")
	}

	content := message["content"]

	// Content can be a string or a list of objects.
	switch v := content.(type) {
	case string:
		return v, nil
	case []interface{}:
		var pieces []string
		for _, item := range v {
			if obj, ok := item.(map[string]interface{}); ok {
				if text, ok := obj["text"].(string); ok && text != "" {
					pieces = append(pieces, text)
				}
				if text, ok := obj["content"].(string); ok && text != "" {
					pieces = append(pieces, text)
				}
			}
		}
		if len(pieces) > 0 {
			return strings.Join(pieces, "\n"), nil
		}
	}

	return "", fmt.Errorf("could not extract transcript text from MiMo response")
}

type httpClientError struct {
	statusCode int
	message    string
	retryAfter time.Duration // parsed Retry-After header, 0 if absent/invalid
}

func (e *httpClientError) Error() string {
	return e.message
}

// isRetriable returns true for rate-limit (429) and request-timeout (408) errors.
func (e *httpClientError) isRetriable() bool {
	return e.statusCode == http.StatusTooManyRequests || e.statusCode == http.StatusRequestTimeout
}

func isHTTPClientError(err error) bool {
	_, ok := err.(*httpClientError)
	return ok
}

func isRetriableHTTPError(err error) bool {
	if herr, ok := err.(*httpClientError); ok {
		return herr.isRetriable()
	}
	return false
}

func isCompleteFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// retryBackoff computes the backoff duration for a retry attempt.
// If the error carries a Retry-After header (e.g. 429), that value is used.
// Otherwise, exponential backoff capped at 10 seconds is used.
func retryBackoff(err error, attempt int) time.Duration {
	if herr, ok := err.(*httpClientError); ok && herr.retryAfter > 0 {
		return herr.retryAfter
	}
	backoff := 1 << uint(attempt) // 2^attempt
	if backoff > 10 {
		backoff = 10
	}
	return time.Duration(backoff) * time.Second
}

// parseRetryAfter parses a Retry-After header value (seconds or HTTP-date).
// Returns 0 if the value is absent or unparseable.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	// Try integer seconds first.
	if secs, err := strconv.Atoi(value); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	// Try HTTP-date format.
	if t, err := http.ParseTime(value); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
