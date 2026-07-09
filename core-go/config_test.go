package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `MIMO_API_KEY=test-key-123
MIMO_ASR_ENDPOINT=https://custom.example.com/v1/chat/completions
MAX_UPLOAD_MB=20
DEFAULT_CHUNK_MINUTES=3
# This is a comment
MIMO_RETRY_COUNT=5

MIMO_ASR_MODEL=custom-model
MIMO_TIMEOUT_SECONDS=60
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings(envPath)
	if err != nil {
		t.Fatal(err)
	}

	if s.MimoAPIKey != "test-key-123" {
		t.Errorf("MimoAPIKey = %q, want %q", s.MimoAPIKey, "test-key-123")
	}
	if s.MimoEndpoint != "https://custom.example.com/v1/chat/completions" {
		t.Errorf("MimoEndpoint = %q", s.MimoEndpoint)
	}
	if s.MaxUploadMB != 20 {
		t.Errorf("MaxUploadMB = %v, want 20", s.MaxUploadMB)
	}
	if s.DefaultChunkMin != 3 {
		t.Errorf("DefaultChunkMin = %v, want 3", s.DefaultChunkMin)
	}
	if s.RetryCount != 5 {
		t.Errorf("RetryCount = %v, want 5", s.RetryCount)
	}
	if s.MimoModel != "custom-model" {
		t.Errorf("MimoModel = %q", s.MimoModel)
	}
	if s.TimeoutSeconds != 60 {
		t.Errorf("TimeoutSeconds = %v, want 60", s.TimeoutSeconds)
	}
}

func TestLoadSettingsDefaults(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=sk-test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings(envPath)
	if err != nil {
		t.Fatal(err)
	}

	if s.MimoEndpoint != defaultMimoEndpoint {
		t.Errorf("default endpoint = %q, want %q", s.MimoEndpoint, defaultMimoEndpoint)
	}
	if s.MimoModel != defaultMimoModel {
		t.Errorf("default model = %q, want %q", s.MimoModel, defaultMimoModel)
	}
	if s.MaxUploadMB != defaultMaxUploadMB {
		t.Errorf("default MaxUploadMB = %v, want %v", s.MaxUploadMB, defaultMaxUploadMB)
	}
	if s.DefaultChunkMin != defaultChunkMinutes {
		t.Errorf("default ChunkMin = %v, want %v", s.DefaultChunkMin, defaultChunkMinutes)
	}
}

func TestLoadSettingsMissingFile(t *testing.T) {
	_, err := LoadSettings("/nonexistent/.env")
	if err == nil {
		t.Fatal("expected error for missing .env file")
	}
}

func TestLoadSettingsInvalidNumber(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=x\nMAX_UPLOAD_MB=notanumber\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSettings(envPath)
	if err == nil {
		t.Fatal("expected error for invalid MAX_UPLOAD_MB")
	}
	if !strings.Contains(err.Error(), "MAX_UPLOAD_MB") {
		t.Errorf("error should mention MAX_UPLOAD_MB, got: %v", err)
	}
}

func TestParseEnvFileQuotedValues(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `MIMO_API_KEY=plain-value
MIMO_ASR_ENDPOINT="https://quoted.example.com/v1"
MIMO_ASR_MODEL='single-quoted-model'
MAX_UPLOAD_MB=15
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings(envPath)
	if err != nil {
		t.Fatal(err)
	}

	if s.MimoAPIKey != "plain-value" {
		t.Errorf("plain value: got %q, want %q", s.MimoAPIKey, "plain-value")
	}
	if s.MimoEndpoint != "https://quoted.example.com/v1" {
		t.Errorf("double-quoted value: got %q, want %q", s.MimoEndpoint, "https://quoted.example.com/v1")
	}
	if s.MimoModel != "single-quoted-model" {
		t.Errorf("single-quoted value: got %q, want %q", s.MimoModel, "single-quoted-model")
	}
	if s.MaxUploadMB != 15 {
		t.Errorf("MaxUploadMB = %v, want 15", s.MaxUploadMB)
	}
}

func TestParseEnvFileCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `
# Top comment
MIMO_API_KEY=key1

  # Indented comment
  	MIMO_ASR_MODEL=model1

# Another comment
MIMO_RETRY_COUNT=2

`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings(envPath)
	if err != nil {
		t.Fatal(err)
	}

	if s.MimoAPIKey != "key1" {
		t.Errorf("MimoAPIKey = %q, want %q", s.MimoAPIKey, "key1")
	}
	if s.MimoModel != "model1" {
		t.Errorf("MimoModel = %q, want %q", s.MimoModel, "model1")
	}
	if s.RetryCount != 2 {
		t.Errorf("RetryCount = %v, want 2", s.RetryCount)
	}
}

func TestLoadSettingsNegativeValues(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=x\nMAX_UPLOAD_MB=-5\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSettings(envPath)
	if err == nil {
		t.Fatal("expected error for negative MAX_UPLOAD_MB")
	}
}

func TestLoadSettingsZeroValues(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=x\nDEFAULT_CHUNK_MINUTES=0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSettings(envPath)
	if err == nil {
		t.Fatal("expected error for zero DEFAULT_CHUNK_MINUTES")
	}
}

func TestFindEnvFile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	found, ok := FindEnvFile(sub)
	if !ok {
		t.Fatal("expected to find .env")
	}
	if found != envPath {
		t.Errorf("found %q, want %q", found, envPath)
	}
}

func TestFindEnvFileNotFound(t *testing.T) {
	_, ok := FindEnvFile("/tmp")
	if ok {
		t.Fatal("should not find .env in /tmp")
	}
}

func TestEnvVarPrecedence(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MIMO_API_KEY=from-file\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MIMO_API_KEY", "from-env")

	s, err := LoadSettings(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if s.MimoAPIKey != "from-env" {
		t.Errorf("MimoAPIKey = %q, want %q (env should take precedence)", s.MimoAPIKey, "from-env")
	}
}
