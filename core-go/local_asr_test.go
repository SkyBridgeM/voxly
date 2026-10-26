package main

import "testing"

func TestLocalASRLanguageSelection(t *testing.T) {
	for input, want := range map[string]string{"": "zh", "zh": "zh", "auto": "auto"} {
		actual, err := normalizeLocalASRLanguage(input)
		if err != nil || actual != want {
			t.Fatalf("language %q = %q, %v", input, actual, err)
		}
		settings := DefaultSettings()
		settings.LocalASRLanguage = input
		provider, err := ResolveASRProvider(ASRProviderConfig{ProviderID: ASRProviderLocalSenseVoice, ModelID: ASRModelSenseVoiceSmallINT8}, settings, "")
		if err != nil || provider.(localASRProvider).language != want {
			t.Fatalf("provider language %q: %v", input, err)
		}
	}
	settings := DefaultSettings()
	settings.LocalASRLanguage = "invalid"
	if _, err := ResolveASRProvider(ASRProviderConfig{ProviderID: ASRProviderLocalSenseVoice, ModelID: ASRModelSenseVoiceSmallINT8}, settings, ""); err == nil {
		t.Fatal("invalid language accepted")
	}
}
