package main

import "fmt"

// ValidateMinutesProvider checks that the configured provider is supported.
// Call this early (before expensive work) when output-mode=minutes.
func ValidateMinutesProvider(settings *Settings) error {
	provider := settings.MinutesProvider
	if provider == "" || provider == "deepseek" {
		return nil
	}
	return fmt.Errorf("output-mode=minutes requires MINUTES_PROVIDER=deepseek, got %q", provider)
}

// GenerateMinutes dispatches to the configured minutes provider.
// This is the single entry point for meeting-minutes generation.
func GenerateMinutes(notesInputPath string, settings *Settings) (string, error) {
	if err := ValidateMinutesProvider(settings); err != nil {
		return "", err
	}

	provider := settings.MinutesProvider
	if provider == "" {
		provider = "deepseek"
	}

	switch provider {
	case "deepseek":
		return generateDeepSeekMinutes(notesInputPath, settings)
	default:
		return "", fmt.Errorf("output-mode=minutes requires MINUTES_PROVIDER=deepseek, got %q", provider)
	}
}
