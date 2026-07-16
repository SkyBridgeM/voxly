package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuiltInProviderCatalog(t *testing.T) {
	catalog := BuiltInProviderCatalog()
	if got := providerDescriptorIDs(catalog.ASR); !reflect.DeepEqual(got, []string{"mimo"}) {
		t.Fatalf("ASR providers = %v", got)
	}
	if got := providerDescriptorIDs(catalog.LLM); !reflect.DeepEqual(got, []string{"deepseek"}) {
		t.Fatalf("LLM providers = %v", got)
	}
	for _, descriptor := range append(catalog.ASR, catalog.LLM...) {
		if !descriptor.BuiltIn || descriptor.DefaultModel == "" || len(descriptor.Models) != 1 || descriptor.Models[0].ID != descriptor.DefaultModel {
			t.Fatalf("invalid descriptor: %#v", descriptor)
		}
		encoded := descriptor.ID + descriptor.DefaultModel + descriptor.DisplayName["en"]
		if strings.Contains(strings.ToLower(encoded), "api_key") || strings.Contains(strings.ToLower(encoded), "secret") {
			t.Fatalf("descriptor contains credential metadata: %#v", descriptor)
		}
	}
}

func TestProviderConfigValidationAndReadiness(t *testing.T) {
	if err := ValidateASRProviderConfig(DefaultASRProviderConfig()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLLMProviderConfig(DefaultLLMProviderConfig()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateASRProviderConfig(ASRProviderConfig{ProviderID: "other", ModelID: ASRModelMimoV25}); err == nil {
		t.Fatal("unknown ASR provider accepted")
	}
	if err := ValidateASRProviderConfig(ASRProviderConfig{ProviderID: ASRProviderMimo, ModelID: "other"}); err == nil {
		t.Fatal("unsupported ASR model accepted")
	}
	if err := ValidateLLMProviderConfig(LLMProviderConfig{ProviderID: "other", ModelID: LLMModelDeepSeekV4}); err == nil {
		t.Fatal("unknown LLM provider accepted")
	}
	if err := ValidateLLMProviderConfig(LLMProviderConfig{ProviderID: LLMProviderDeepSeek, ModelID: "other"}); err == nil {
		t.Fatal("unsupported LLM model accepted")
	}
	if !EvaluateASRReadiness(DefaultASRProviderConfig(), true).Ready || EvaluateASRReadiness(DefaultASRProviderConfig(), false).Ready {
		t.Fatal("ASR readiness incorrect")
	}
	if !EvaluateLLMReadiness(DefaultLLMProviderConfig(), true).Ready || EvaluateLLMReadiness(DefaultLLMProviderConfig(), false).Ready {
		t.Fatal("LLM readiness incorrect")
	}
	capabilities, err := ResolveASRCapabilities(DefaultASRProviderConfig())
	if err != nil || capabilities.ProviderID != "mimo" || capabilities.ModelID != "mimo-v2.5-asr" {
		t.Fatalf("ASR resolution = %#v, %v", capabilities, err)
	}
	if _, err := ResolveLLMContentGenerator(DefaultLLMProviderConfig()); err != nil {
		t.Fatalf("LLM resolution failed: %v", err)
	}
}

func TestManifestSnapshotsEffectiveASRProviderConfiguration(t *testing.T) {
	settings := DefaultSettings()
	m := CreateManifest(t.TempDir(), "audio.wav", "transcript_only", 5, 10, settings.ASRProvider, settings.MimoModel)
	if m.Chunks.ASRProvider != string(ASRProviderMimo) || m.Chunks.ASRModel != string(ASRModelMimoV25) {
		t.Fatalf("chunk provider snapshot = %s/%s", m.Chunks.ASRProvider, m.Chunks.ASRModel)
	}
	if m.CanonicalTranscript == nil || m.CanonicalTranscript.ASRProvider != string(ASRProviderMimo) || m.CanonicalTranscript.ASRModel != string(ASRModelMimoV25) {
		t.Fatalf("canonical provider snapshot = %#v", m.CanonicalTranscript)
	}
}
