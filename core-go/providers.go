package main

import (
	"fmt"
	"sort"
)

type ASRProviderID string
type LLMProviderID string
type ASRModelID string
type LLMModelID string

const (
	ASRProviderMimo     ASRProviderID = "mimo"
	LLMProviderDeepSeek LLMProviderID = "deepseek"
	ASRModelMimoV25     ASRModelID    = "mimo-v2.5-asr"
	LLMModelDeepSeekV4  LLMModelID    = "deepseek-v4-flash"
)

type ProviderModelDescriptor struct {
	ID          string            `json:"id"`
	DisplayName map[string]string `json:"display_name"`
}

type ProviderDescriptor struct {
	Kind             string                    `json:"kind"`
	ID               string                    `json:"id"`
	DisplayName      map[string]string         `json:"display_name"`
	Models           []ProviderModelDescriptor `json:"models"`
	DefaultModel     string                    `json:"default_model"`
	CredentialKey    string                    `json:"credential_key"`
	AllowCustomModel bool                      `json:"allow_custom_model"`
	BuiltIn          bool                      `json:"built_in"`
}

type ProviderCatalog struct {
	ASR []ProviderDescriptor `json:"asr"`
	LLM []ProviderDescriptor `json:"llm"`
}

type ASRProviderConfig struct {
	ProviderID ASRProviderID `json:"provider_id"`
	ModelID    ASRModelID    `json:"model_id"`
}

type LLMProviderConfig struct {
	ProviderID LLMProviderID `json:"provider_id"`
	ModelID    LLMModelID    `json:"model_id"`
}

type ProviderReadiness struct {
	ProviderID      string   `json:"provider_id"`
	ModelID         string   `json:"model_id"`
	Ready           bool     `json:"ready"`
	BlockingReasons []string `json:"blocking_reasons"`
	Warnings        []string `json:"warnings"`
}

func BuiltInProviderCatalog() ProviderCatalog {
	return ProviderCatalog{
		ASR: []ProviderDescriptor{{
			Kind: "asr", ID: string(ASRProviderMimo),
			DisplayName:  map[string]string{"en": "Mimo", "zh": "Mimo"},
			Models:       []ProviderModelDescriptor{{ID: string(ASRModelMimoV25), DisplayName: map[string]string{"en": "Mimo v2.5 ASR", "zh": "Mimo v2.5 ASR"}}},
			DefaultModel: string(ASRModelMimoV25), CredentialKey: "MIMO_API_KEY", AllowCustomModel: false, BuiltIn: true,
		}},
		LLM: []ProviderDescriptor{{
			Kind: "llm", ID: string(LLMProviderDeepSeek),
			DisplayName:  map[string]string{"en": "DeepSeek", "zh": "DeepSeek"},
			Models:       []ProviderModelDescriptor{{ID: string(LLMModelDeepSeekV4), DisplayName: map[string]string{"en": "DeepSeek V4 Flash", "zh": "DeepSeek V4 Flash"}}},
			DefaultModel: string(LLMModelDeepSeekV4), CredentialKey: "DEEPSEEK_API_KEY", AllowCustomModel: false, BuiltIn: true,
		}},
	}
}

func DefaultASRProviderConfig() ASRProviderConfig {
	return ASRProviderConfig{ProviderID: ASRProviderMimo, ModelID: ASRModelMimoV25}
}

func DefaultLLMProviderConfig() LLMProviderConfig {
	return LLMProviderConfig{ProviderID: LLMProviderDeepSeek, ModelID: LLMModelDeepSeekV4}
}

func ValidateASRProviderConfig(config ASRProviderConfig) error {
	if config.ProviderID != ASRProviderMimo {
		return fmt.Errorf("unknown ASR provider %q", config.ProviderID)
	}
	if config.ModelID != ASRModelMimoV25 {
		return fmt.Errorf("ASR model %q is not supported by provider %q", config.ModelID, config.ProviderID)
	}
	return nil
}

func ValidateLLMProviderConfig(config LLMProviderConfig) error {
	if config.ProviderID != LLMProviderDeepSeek {
		return fmt.Errorf("unknown LLM provider %q", config.ProviderID)
	}
	if config.ModelID != LLMModelDeepSeekV4 {
		return fmt.Errorf("LLM model %q is not supported by provider %q", config.ModelID, config.ProviderID)
	}
	return nil
}

func ResolveASRCapabilities(config ASRProviderConfig) (ASRProviderCapabilities, error) {
	if err := ValidateASRProviderConfig(config); err != nil {
		return ASRProviderCapabilities{}, err
	}
	return MimoCapabilities(string(config.ModelID)), nil
}

func ResolveLLMContentGenerator(config LLMProviderConfig) (ContentGenerator, error) {
	if err := ValidateLLMProviderConfig(config); err != nil {
		return nil, err
	}
	return deepSeekGenerator{}, nil
}

func EvaluateASRReadiness(config ASRProviderConfig, credentialAvailable bool) ProviderReadiness {
	result := ProviderReadiness{ProviderID: string(config.ProviderID), ModelID: string(config.ModelID), BlockingReasons: []string{}, Warnings: []string{}}
	if err := ValidateASRProviderConfig(config); err != nil {
		result.BlockingReasons = append(result.BlockingReasons, err.Error())
	}
	if !credentialAvailable {
		result.BlockingReasons = append(result.BlockingReasons, "MIMO_API_KEY is not configured")
	}
	result.Ready = len(result.BlockingReasons) == 0
	return result
}

func EvaluateLLMReadiness(config LLMProviderConfig, credentialAvailable bool) ProviderReadiness {
	result := ProviderReadiness{ProviderID: string(config.ProviderID), ModelID: string(config.ModelID), BlockingReasons: []string{}, Warnings: []string{}}
	if err := ValidateLLMProviderConfig(config); err != nil {
		result.BlockingReasons = append(result.BlockingReasons, err.Error())
	}
	if !credentialAvailable {
		result.BlockingReasons = append(result.BlockingReasons, "DEEPSEEK_API_KEY is not configured")
	}
	result.Ready = len(result.BlockingReasons) == 0
	return result
}

func (s *Settings) ASRConfig() ASRProviderConfig {
	return ASRProviderConfig{ProviderID: ASRProviderID(s.ASRProvider), ModelID: ASRModelID(s.MimoModel)}
}

func (s *Settings) LLMConfig() LLMProviderConfig {
	return LLMProviderConfig{ProviderID: LLMProviderID(s.LLMProvider), ModelID: LLMModelID(s.DeepSeekModel)}
}

func providerDescriptorIDs(descriptors []ProviderDescriptor) []string {
	ids := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		ids = append(ids, descriptor.ID)
	}
	sort.Strings(ids)
	return ids
}
