package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDeepSeekContentString(t *testing.T) {
	body := `{"choices":[{"message":{"content":"Meeting summary here."}}]}`
	got, err := extractDeepSeekContent([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if got != "Meeting summary here." {
		t.Errorf("got %q, want %q", got, "Meeting summary here.")
	}
}

func TestExtractDeepSeekContentEmpty(t *testing.T) {
	body := `{"choices":[{"message":{"content":""}}]}`
	_, err := extractDeepSeekContent([]byte(body))
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if !strings.Contains(err.Error(), "empty content") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractDeepSeekContentMissingChoices(t *testing.T) {
	body := `{"usage":{}}`
	_, err := extractDeepSeekContent([]byte(body))
	if err == nil {
		t.Fatal("expected error for missing choices")
	}
	if !strings.Contains(err.Error(), "missing choices") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractDeepSeekContentInvalidJSON(t *testing.T) {
	_, err := extractDeepSeekContent([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractDeepSeekContentWhitespaceOnly(t *testing.T) {
	body := `{"choices":[{"message":{"content":"   \n  "}}]}`
	_, err := extractDeepSeekContent([]byte(body))
	if err == nil {
		t.Fatal("expected error for whitespace-only content")
	}
	if !strings.Contains(err.Error(), "empty content") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractDeepSeekCompletionReportsFinishReason(t *testing.T) {
	truncated := `{"choices":[{"finish_reason":"length","message":{"content":"partial text"}}]}`
	outcome, err := extractDeepSeekCompletion([]byte(truncated))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Text != "partial text" || outcome.FinishReason != "length" {
		t.Fatalf("outcome = %#v", outcome)
	}

	empty := `{"choices":[{"finish_reason":"length","message":{"content":"   "}}]}`
	outcome, err = extractDeepSeekCompletion([]byte(empty))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Text != "" || outcome.FinishReason != "length" {
		t.Fatalf("empty truncated outcome = %#v", outcome)
	}
	if _, err := extractDeepSeekContent([]byte(empty)); err == nil || !strings.Contains(err.Error(), "empty content") {
		t.Fatalf("plain text contract error = %v", err)
	}

	unreported := `{"choices":[{"message":{"content":"no reason"}}]}`
	outcome, err = extractDeepSeekCompletion([]byte(unreported))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Text != "no reason" || outcome.FinishReason != "" {
		t.Fatalf("unreported outcome = %#v", outcome)
	}

	// A refusal is allowed to carry no body at all, and must keep its own class
	// instead of collapsing into a generic provider failure.
	refused := `{"choices":[{"finish_reason":"content_filter","message":{"content":null}}]}`
	outcome, err = extractDeepSeekCompletion([]byte(refused))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Text != "" || outcome.FinishReason != "content_filter" {
		t.Fatalf("refused outcome = %#v", outcome)
	}

	// A normal stop still requires text, and a wrongly typed body stays a parse
	// error rather than a completion class.
	stopped := `{"choices":[{"finish_reason":"stop","message":{"content":"  "}}]}`
	if _, err := extractDeepSeekCompletion([]byte(stopped)); err == nil || !strings.Contains(err.Error(), "empty content") {
		t.Fatalf("empty normal stop error = %v", err)
	}
	malformed := `{"choices":[{"finish_reason":"stop","message":{"content":42}}]}`
	if _, err := extractDeepSeekCompletion([]byte(malformed)); err == nil || !strings.Contains(err.Error(), "not a string") {
		t.Fatalf("malformed content error = %v", err)
	}
}

func TestExtractDeepSeekError(t *testing.T) {
	body := `{"error":{"message":"Invalid API key"}}`
	got := extractDeepSeekError([]byte(body))
	if got != "Invalid API key" {
		t.Errorf("got %q, want %q", got, "Invalid API key")
	}
}

func TestExtractDeepSeekErrorFallback(t *testing.T) {
	body := `some random text`
	got := extractDeepSeekError([]byte(body))
	if got != "some random text" {
		t.Errorf("got %q, want %q", got, "some random text")
	}
}

func TestCheckDeepSeekStatus(t *testing.T) {
	tests := []struct {
		code    int
		wantErr string
	}{
		{200, ""},
		{401, "authentication failed"},
		{402, "account balance"},
		{429, "rate limit"},
		{400, "client error"},
		{500, "server error"},
	}
	body := []byte(`{"error":{"message":"test"}}`)
	for _, tt := range tests {
		err := checkDeepSeekStatus(tt.code, body)
		if tt.wantErr == "" {
			if err != nil {
				t.Errorf("status %d: unexpected error: %v", tt.code, err)
			}
		} else {
			if err == nil {
				t.Errorf("status %d: expected error containing %q", tt.code, tt.wantErr)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("status %d: error %q should contain %q", tt.code, err, tt.wantErr)
			}
		}
	}
}

func TestGenerateMinutesIntegration(t *testing.T) {
	// Start a mock DeepSeek server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure.
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"message":"unauthorized"}}`))
			return
		}

		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "## Meeting Summary\n\nKey decisions: none."}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	dir := t.TempDir()
	notesPath := filepath.Join(dir, "notes_input.md")
	os.WriteFile(notesPath, []byte("prompt\n\n---\n\n## Transcript\n\nHello."), 0644)

	settings := DefaultSettings()
	settings.DeepSeekAPIKey = "test-key"
	settings.DeepSeekEndpoint = server.URL
	settings.DeepSeekModel = "test-model"
	settings.LLMTimeoutSeconds = 10

	got, err := GenerateMinutes(notesPath, settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Meeting Summary") {
		t.Errorf("response should contain summary, got: %q", got)
	}
}

func TestGenerateMinutesMissingKey(t *testing.T) {
	dir := t.TempDir()
	notesPath := filepath.Join(dir, "notes_input.md")
	os.WriteFile(notesPath, []byte("content"), 0644)

	settings := DefaultSettings()
	settings.DeepSeekAPIKey = ""

	_, err := GenerateMinutes(notesPath, settings)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "DEEPSEEK_API_KEY") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateMinutesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"internal error"}}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	notesPath := filepath.Join(dir, "notes_input.md")
	os.WriteFile(notesPath, []byte("content"), 0644)

	settings := DefaultSettings()
	settings.DeepSeekAPIKey = "key"
	settings.DeepSeekEndpoint = server.URL
	settings.LLMTimeoutSeconds = 5

	_, err := GenerateMinutes(notesPath, settings)
	if err == nil {
		t.Fatal("expected error for server error")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateMinutesAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	notesPath := filepath.Join(dir, "notes_input.md")
	os.WriteFile(notesPath, []byte("content"), 0644)

	settings := DefaultSettings()
	settings.DeepSeekAPIKey = "bad-key"
	settings.DeepSeekEndpoint = server.URL
	settings.LLMTimeoutSeconds = 5

	_, err := GenerateMinutes(notesPath, settings)
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateMinutesEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	notesPath := filepath.Join(dir, "notes_input.md")
	os.WriteFile(notesPath, []byte("content"), 0644)

	settings := DefaultSettings()
	settings.DeepSeekAPIKey = "key"
	settings.DeepSeekEndpoint = server.URL
	settings.LLMTimeoutSeconds = 5

	_, err := GenerateMinutes(notesPath, settings)
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if !strings.Contains(err.Error(), "empty content") {
		t.Errorf("unexpected error: %v", err)
	}
}
