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
	defaultMimoEndpoint         = "https://api.xiaomimimo.com/v1/chat/completions"
	defaultMimoModel            = "mimo-v2.5-asr"
	defaultMaxUploadMB          = 10.0
	defaultChunkMinutes         = 5
	defaultRetryCount           = 3
	defaultTimeoutSeconds       = 120
	minChunkSeconds             = 60
	defaultMinutesProvider      = "deepseek"
	defaultDeepSeekEndpoint     = "https://api.deepseek.com/chat/completions"
	defaultDeepSeekModel        = "deepseek-v4-flash"
	defaultLLMTimeoutSeconds    = 120
	defaultMinutesPromptPath    = "prompts/default_minutes.md"
	defaultObsidianTemplatePath = "prompts/obsidian_minutes.md"
)

var supportedAudioSuffixes = map[string]bool{
	".m4a": true,
	".mp3": true,
	".wav": true,
}

type Settings struct {
	ASRProvider          string
	MimoAPIKey           string
	MimoEndpoint         string
	MimoModel            string
	MaxUploadMB          float64
	DefaultChunkMin      int
	RetryCount           int
	TimeoutSeconds       int
	MinutesProvider      string
	LLMProvider          string
	DeepSeekAPIKey       string
	DeepSeekEndpoint     string
	DeepSeekModel        string
	LLMTimeoutSeconds    int
	MinutesPromptPath    string
	ObsidianTemplatePath string
	TemplateStorePath    string
	AppDataRoot          string
	// EnvBase is the directory containing the .env file, used to resolve relative paths.
	EnvBase string
}

func DefaultSettings() *Settings {
	return &Settings{
		ASRProvider:          string(ASRProviderMimo),
		MimoEndpoint:         defaultMimoEndpoint,
		MimoModel:            defaultMimoModel,
		MaxUploadMB:          defaultMaxUploadMB,
		DefaultChunkMin:      defaultChunkMinutes,
		RetryCount:           defaultRetryCount,
		TimeoutSeconds:       defaultTimeoutSeconds,
		MinutesProvider:      defaultMinutesProvider,
		LLMProvider:          string(LLMProviderDeepSeek),
		DeepSeekEndpoint:     defaultDeepSeekEndpoint,
		DeepSeekModel:        defaultDeepSeekModel,
		LLMTimeoutSeconds:    defaultLLMTimeoutSeconds,
		MinutesPromptPath:    defaultMinutesPromptPath,
		ObsidianTemplatePath: defaultObsidianTemplatePath,
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

func (s *Settings) RequireDeepSeekAPIKey() (string, error) {
	if s.DeepSeekAPIKey == "" {
		return "", fmt.Errorf("DEEPSEEK_API_KEY is not configured. Copy .env.example to .env and set DEEPSEEK_API_KEY")
	}
	return s.DeepSeekAPIKey, nil
}

// ResolvePath resolves a path relative to EnvBase if it is not absolute.
func (s *Settings) ResolvePath(p string) string {
	if filepath.IsAbs(p) || s.EnvBase == "" {
		return p
	}
	return filepath.Join(s.EnvBase, p)
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

	// Set EnvBase to the directory containing the .env file.
	absEnv, err := filepath.Abs(envPath)
	if err == nil {
		s.EnvBase = filepath.Dir(absEnv)
	}

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

	// DeepSeek / minutes settings.
	if v, ok := vars["MINUTES_PROVIDER"]; ok {
		s.MinutesProvider = v
	}
	if v, ok := vars["DEEPSEEK_API_KEY"]; ok {
		s.DeepSeekAPIKey = v
	}
	if v, ok := vars["DEEPSEEK_ENDPOINT"]; ok {
		s.DeepSeekEndpoint = v
	}
	if v, ok := vars["DEEPSEEK_MODEL"]; ok {
		s.DeepSeekModel = v
	}
	if v, ok := vars["LLM_TIMEOUT_SECONDS"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("LLM_TIMEOUT_SECONDS must be an integer, got %q", v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("LLM_TIMEOUT_SECONDS must be greater than 0, got %v", n)
		}
		s.LLMTimeoutSeconds = n
	}
	if v, ok := vars["MINUTES_PROMPT_PATH"]; ok {
		s.MinutesPromptPath = v
	}
	if v, ok := vars["OBSIDIAN_TEMPLATE_PATH"]; ok {
		s.ObsidianTemplatePath = v
	}

	// Override from environment variables (env vars take precedence over .env file).
	if v := os.Getenv("MIMO_API_KEY"); v != "" {
		s.MimoAPIKey = v
	}
	if v := os.Getenv("MIMO_ASR_ENDPOINT"); v != "" {
		s.MimoEndpoint = v
	}
	if v := os.Getenv("MIMO_ASR_MODEL"); v != "" {
		s.MimoModel = v
	}
	if v := os.Getenv("MAX_UPLOAD_MB"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("env MAX_UPLOAD_MB must be a number, got %q", v)
		}
		if f <= 0 {
			return nil, fmt.Errorf("env MAX_UPLOAD_MB must be greater than 0, got %v", f)
		}
		s.MaxUploadMB = f
	}
	if v := os.Getenv("DEFAULT_CHUNK_MINUTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("env DEFAULT_CHUNK_MINUTES must be an integer, got %q", v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("env DEFAULT_CHUNK_MINUTES must be greater than 0, got %v", n)
		}
		s.DefaultChunkMin = n
	}
	if v := os.Getenv("MIMO_RETRY_COUNT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("env MIMO_RETRY_COUNT must be an integer, got %q", v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("env MIMO_RETRY_COUNT must be greater than 0, got %v", n)
		}
		s.RetryCount = n
	}
	if v := os.Getenv("MIMO_TIMEOUT_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("env MIMO_TIMEOUT_SECONDS must be an integer, got %q", v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("env MIMO_TIMEOUT_SECONDS must be greater than 0, got %v", n)
		}
		s.TimeoutSeconds = n
	}
	if v := os.Getenv("MINUTES_PROVIDER"); v != "" {
		s.MinutesProvider = v
	}
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		s.DeepSeekAPIKey = v
	}
	if v := os.Getenv("DEEPSEEK_ENDPOINT"); v != "" {
		s.DeepSeekEndpoint = v
	}
	if v := os.Getenv("DEEPSEEK_MODEL"); v != "" {
		s.DeepSeekModel = v
	}
	if v := os.Getenv("LLM_TIMEOUT_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("env LLM_TIMEOUT_SECONDS must be an integer, got %q", v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("env LLM_TIMEOUT_SECONDS must be greater than 0, got %v", n)
		}
		s.LLMTimeoutSeconds = n
	}
	if v := os.Getenv("MINUTES_PROMPT_PATH"); v != "" {
		s.MinutesPromptPath = v
	}
	if v := os.Getenv("OBSIDIAN_TEMPLATE_PATH"); v != "" {
		s.ObsidianTemplatePath = v
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
