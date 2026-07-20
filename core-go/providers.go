package main

import (
	"context"
	"fmt"
	"sort"
)

type ASRProviderID string
type LLMProviderID string
type ASRModelID string
type LLMModelID string

const (
	ASRProviderMimo             ASRProviderID = "mimo"
	ASRProviderLocalSenseVoice  ASRProviderID = "local-sensevoice"
	LLMProviderDeepSeek         LLMProviderID = "deepseek"
	ASRModelMimoV25             ASRModelID    = "mimo-v2.5-asr"
	ASRModelSenseVoiceSmallINT8 ASRModelID    = "sensevoice-small-int8"
	LLMModelDeepSeekV4          LLMModelID    = "deepseek-v4-flash"
)

type ProviderModelDescriptor struct {
	ID          string            `json:"id"`
	DisplayName map[string]string `json:"display_name"`
}

type ProviderDescriptor struct {
	Kind                      string                    `json:"kind"`
	ID                        string                    `json:"id"`
	DisplayName               map[string]string         `json:"display_name"`
	Models                    []ProviderModelDescriptor `json:"models"`
	DefaultModel              string                    `json:"default_model"`
	CredentialKey             string                    `json:"credential_key"`
	AllowCustomModel          bool                      `json:"allow_custom_model"`
	BuiltIn                   bool                      `json:"built_in"`
	ExecutionLocation         string                    `json:"execution_location"`
	RuntimeKind               string                    `json:"runtime_kind"`
	RequiresCredentials       bool                      `json:"requires_credentials"`
	RequiresExternalService   bool                      `json:"requires_external_service"`
	ModelSource               string                    `json:"model_source"`
	SupportsModelInstallation bool                      `json:"supports_model_installation"`
	SupportsManualImport      bool                      `json:"supports_manual_import"`
	Privacy                   ProviderPrivacyFacts      `json:"privacy"`
}

type ProviderPrivacyFacts struct {
	SendsAudioOffDevice bool `json:"sends_audio_off_device"`
	SendsTextOffDevice  bool `json:"sends_text_off_device"`
}

type PrivacyState struct {
	Kind                   string            `json:"kind"`
	SendsAudioOffDevice    bool              `json:"sends_audio_off_device"`
	SendsTextOffDevice     bool              `json:"sends_text_off_device"`
	ContentGenerationLocal bool              `json:"content_generation_local"`
	ASRCredentialsRequired bool              `json:"asr_credentials_required"`
	LLMCredentialsRequired bool              `json:"llm_credentials_required"`
	Title                  map[string]string `json:"title"`
	Description            map[string]string `json:"description"`
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
			ExecutionLocation: "cloud", RuntimeKind: "remote_http", RequiresCredentials: true, RequiresExternalService: true, ModelSource: "static_catalog",
			Privacy: ProviderPrivacyFacts{SendsAudioOffDevice: true},
		}, {
			Kind: "asr", ID: string(ASRProviderLocalSenseVoice),
			DisplayName:  map[string]string{"en": "Local speech recognition", "zh": "本地语音识别"},
			Models:       []ProviderModelDescriptor{{ID: string(ASRModelSenseVoiceSmallINT8), DisplayName: map[string]string{"en": "Chinese-first · SenseVoiceSmall INT8", "zh": "中文为主 · SenseVoiceSmall INT8"}}},
			DefaultModel: string(ASRModelSenseVoiceSmallINT8), AllowCustomModel: false, BuiltIn: true,
			ExecutionLocation: "local", RuntimeKind: "bundled_sidecar", RequiresCredentials: false, RequiresExternalService: false, ModelSource: "installed_models",
			SupportsModelInstallation: true, SupportsManualImport: true,
			Privacy: ProviderPrivacyFacts{},
		}},
		LLM: []ProviderDescriptor{{
			Kind: "llm", ID: string(LLMProviderDeepSeek),
			DisplayName:  map[string]string{"en": "DeepSeek", "zh": "DeepSeek"},
			Models:       []ProviderModelDescriptor{{ID: string(LLMModelDeepSeekV4), DisplayName: map[string]string{"en": "DeepSeek V4 Flash", "zh": "DeepSeek V4 Flash"}}},
			DefaultModel: string(LLMModelDeepSeekV4), CredentialKey: "DEEPSEEK_API_KEY", AllowCustomModel: false, BuiltIn: true,
			ExecutionLocation: "cloud", RuntimeKind: "remote_http", RequiresCredentials: true, RequiresExternalService: true, ModelSource: "static_catalog",
			Privacy: ProviderPrivacyFacts{SendsTextOffDevice: true},
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
	expected := map[ASRProviderID]ASRModelID{ASRProviderMimo: ASRModelMimoV25, ASRProviderLocalSenseVoice: ASRModelSenseVoiceSmallINT8}
	model, ok := expected[config.ProviderID]
	if !ok {
		return fmt.Errorf("unknown ASR provider %q", config.ProviderID)
	}
	if config.ModelID != model {
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
	if config.ProviderID == ASRProviderMimo {
		return MimoCapabilities(string(config.ModelID)), nil
	}
	return ASRProviderCapabilities{}, &ASRExecutionError{Code: "runtime_not_implemented", Message: "The local speech runtime is not available yet."}
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
	if config.ProviderID == ASRProviderMimo && !credentialAvailable {
		result.BlockingReasons = append(result.BlockingReasons, "MIMO_API_KEY is not configured")
	}
	if config.ProviderID == ASRProviderLocalSenseVoice {
		result.BlockingReasons = append(result.BlockingReasons, "local model and runtime readiness must be verified")
	}
	result.Ready = len(result.BlockingReasons) == 0
	return result
}

func CalculatePrivacyState(asr, llm ProviderDescriptor) PrivacyState {
	audioOffDevice := asr.Privacy.SendsAudioOffDevice
	textOffDevice := llm.Privacy.SendsTextOffDevice
	kind := "local_local"
	title := map[string]string{"zh": "本地 ASR + 本地 LLM", "en": "Local ASR + local LLM"}
	description := map[string]string{"zh": "音频和转写文本均留在本机，生成内容也在本机完成。", "en": "Audio and transcript text stay on this device, and content is generated locally."}
	if !audioOffDevice && textOffDevice {
		kind = "local_cloud"
		title = map[string]string{"zh": "本地 ASR + 云端 LLM", "en": "Local ASR + cloud LLM"}
		description = map[string]string{"zh": "音频留在本机；转写文本会发送给所选云端内容生成服务。", "en": "Audio stays on this device; transcript text is sent to the selected cloud content-generation service."}
	} else if audioOffDevice && !textOffDevice {
		kind = "cloud_local"
		title = map[string]string{"zh": "云端 ASR + 本地 LLM", "en": "Cloud ASR + local LLM"}
		description = map[string]string{"zh": "音频会发送给所选云端语音识别服务；转写后的内容生成在本机完成。", "en": "Audio is sent to the selected cloud speech-recognition service; content is generated locally after transcription."}
	} else if audioOffDevice && textOffDevice {
		kind = "cloud_cloud"
		title = map[string]string{"zh": "云端 ASR + 云端 LLM", "en": "Cloud ASR + cloud LLM"}
		description = map[string]string{"zh": "音频会发送给云端语音识别服务，转写文本会发送给云端内容生成服务。", "en": "Audio is sent to a cloud speech-recognition service, and transcript text is sent to a cloud content-generation service."}
	}
	return PrivacyState{Kind: kind, SendsAudioOffDevice: audioOffDevice, SendsTextOffDevice: textOffDevice, ContentGenerationLocal: !textOffDevice, ASRCredentialsRequired: asr.RequiresCredentials, LLMCredentialsRequired: llm.RequiresCredentials, Title: title, Description: description}
}

type ASRExecutionError struct{ Code, Message string }

func (e *ASRExecutionError) Error() string { return e.Code + ": " + e.Message }

type ASRRequest struct{ AudioPath, Language string }
type ASRResult struct{ Text string }
type ASRProvider interface {
	Transcribe(context.Context, ASRRequest) (ASRResult, error)
}

func ResolveASRProvider(config ASRProviderConfig, settings *Settings, apiKey string) (ASRProvider, error) {
	if err := ValidateASRProviderConfig(config); err != nil {
		return nil, err
	}
	if config.ProviderID == ASRProviderLocalSenseVoice {
		return unavailableLocalASRProvider{}, nil
	}
	return mimoASRProvider{settings: settings, apiKey: apiKey}, nil
}

type unavailableLocalASRProvider struct{}

func (unavailableLocalASRProvider) Transcribe(context.Context, ASRRequest) (ASRResult, error) {
	return ASRResult{}, &ASRExecutionError{Code: "runtime_not_implemented", Message: "The local speech runtime is not available yet."}
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
