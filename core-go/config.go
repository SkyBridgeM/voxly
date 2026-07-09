package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultMimoEndpoint    = "https://api.xiaomimimo.com/v1/chat/completions"
	defaultMimoModel       = "mimo-v2.5-asr"
	defaultMaxUploadMB     = 10.0
	defaultChunkMinutes    = 5
	defaultRetryCount      = 3
	defaultTimeoutSeconds  = 120
	minChunkSeconds        = 60
)

var supportedAudioSuffixes = map[string]bool{
	".m4a": true,
	".mp3": true,
	".wav": true,
}

type Settings struct {
	MimoAPIKey       string
	MimoEndpoint     string
	MimoModel        string
	MaxUploadMB      float64
	DefaultChunkMin  int
	RetryCount       int
	TimeoutSeconds   int
}

func DefaultSettings() *Settings {
	return &Settings{
		MimoEndpoint:    defaultMimoEndpoint,
		MimoModel:       defaultMimoModel,
		MaxUploadMB:     defaultMaxUploadMB,
		DefaultChunkMin: defaultChunkMinutes,
		RetryCount:      defaultRetryCount,
		TimeoutSeconds:  defaultTimeoutSeconds,
	}
}

func (s *Settings) MaxUploadBytes() int64 {
	return int64(s.MaxUploadMB * 1024 * 1024)
}

func (s *Settings) RequireMimoAPIKey() (string, error) {
	if s.MimoAPIKey == "" {
		return "", fmt.Errorf("MIMO_API_KEY is not configured. Copy .env.example to .env and set MIMO_API_KEY")
	}
	return s.MimoAPIKey, nil
}

// FindEnvFile searches upward from startDir looking for a .env file.
func FindEnvFile(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, ".env")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// LoadSettings reads a .env file and returns Settings with defaults applied.
func LoadSettings(envPath string) (*Settings, error) {
	s := DefaultSettings()

	vars, err := parseEnvFile(envPath)
	if err != nil {
		return nil, err
	}

	if v, ok := vars["MIMO_API_KEY"]; ok {
		s.MimoAPIKey = v
	}
	if v, ok := vars["MIMO_ASR_ENDPOINT"]; ok {
		s.MimoEndpoint = v
	}
	if v, ok := vars["MIMO_ASR_MODEL"]; ok {
		s.MimoModel = v
	}
	if v, ok := vars["MAX_UPLOAD_MB"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("MAX_UPLOAD_MB must be a number, got %q", v)
		}
		if f <= 0 {
			return nil, fmt.Errorf("MAX_UPLOAD_MB must be greater than 0, got %v", f)
		}
		s.MaxUploadMB = f
	}
	if v, ok := vars["DEFAULT_CHUNK_MINUTES"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("DEFAULT_CHUNK_MINUTES must be an integer, got %q", v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("DEFAULT_CHUNK_MINUTES must be greater than 0, got %v", n)
		}
		s.DefaultChunkMin = n
	}
	if v, ok := vars["MIMO_RETRY_COUNT"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("MIMO_RETRY_COUNT must be an integer, got %q", v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("MIMO_RETRY_COUNT must be greater than 0, got %v", n)
		}
		s.RetryCount = n
	}
	if v, ok := vars["MIMO_TIMEOUT_SECONDS"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("MIMO_TIMEOUT_SECONDS must be an integer, got %q", v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("MIMO_TIMEOUT_SECONDS must be greater than 0, got %v", n)
		}
		s.TimeoutSeconds = n
	}

	// Override MIMO_API_KEY from environment if set (env vars take precedence over .env file).
	if envKey := os.Getenv("MIMO_API_KEY"); envKey != "" {
		s.MimoAPIKey = envKey
	}

	return s, nil
}

func parseEnvFile(path string) (map[string]string, error) {
	vars := make(map[string]string)

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open .env file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		value := strings.TrimSpace(line[eqIdx+1:])
		// Strip surrounding quotes.
		if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		vars[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .env file: %w", err)
	}

	return vars, nil
}
