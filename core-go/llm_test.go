package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMinutesProvider_Default(t *testing.T) {
	settings := DefaultSettings()
	if err := ValidateMinutesProvider(settings); err != nil {
		t.Errorf("default provider should be valid, got: %v", err)
	}
}

func TestValidateMinutesProvider_DeepSeek(t *testing.T) {
	settings := DefaultSettings()
	settings.MinutesProvider = "deepseek"
	if err := ValidateMinutesProvider(settings); err != nil {
		t.Errorf("deepseek should be valid, got: %v", err)
	}
}

func TestValidateMinutesProvider_Unsupported(t *testing.T) {
	settings := DefaultSettings()
	settings.MinutesProvider = "openai"
	err := ValidateMinutesProvider(settings)
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	if !strings.Contains(err.Error(), "MINUTES_PROVIDER=deepseek") {
		t.Errorf("should mention required provider, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"openai"`) {
		t.Errorf("should mention actual provider, got: %v", err)
	}
}

func TestGenerateMinutes_DefaultProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"summary"}}]}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	notesPath := filepath.Join(dir, "notes.md")
	os.WriteFile(notesPath, []byte("content"), 0644)

	settings := DefaultSettings()
	settings.DeepSeekAPIKey = "key"
	settings.DeepSeekEndpoint = server.URL
	settings.MinutesProvider = ""

	got, err := GenerateMinutes(notesPath, settings)
	if err != nil {
		t.Fatal(err)
	}
	if got != "summary" {
		t.Errorf("got %q, want %q", got, "summary")
	}
}

func TestGenerateMinutes_ExplicitDeepSeek(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	notesPath := filepath.Join(dir, "notes.md")
	os.WriteFile(notesPath, []byte("content"), 0644)

	settings := DefaultSettings()
	settings.DeepSeekAPIKey = "key"
	settings.DeepSeekEndpoint = server.URL
	settings.MinutesProvider = "deepseek"

	got, err := GenerateMinutes(notesPath, settings)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Errorf("got %q, want %q", got, "ok")
	}
}

func TestGenerateMinutes_UnsupportedProvider(t *testing.T) {
	dir := t.TempDir()
	notesPath := filepath.Join(dir, "notes.md")
	os.WriteFile(notesPath, []byte("content"), 0644)

	settings := DefaultSettings()
	settings.MinutesProvider = "openai"

	_, err := GenerateMinutes(notesPath, settings)
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	if !strings.Contains(err.Error(), "MINUTES_PROVIDER=deepseek") {
		t.Errorf("error should mention required provider, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"openai"`) {
		t.Errorf("error should mention actual provider, got: %v", err)
	}
}
