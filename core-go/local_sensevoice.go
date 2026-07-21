package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yudi/meet-asr/core-go/localruntime"
	"github.com/yudi/meet-asr/core-go/modelcatalog"
	"github.com/yudi/meet-asr/core-go/taskmanifest"
)

type localSenseVoiceProvider struct {
	runtimePath string
	modelPath   string
	tokensPath  string
}

func prepareLocalSenseVoice(settings *Settings) (localSenseVoiceProvider, *taskmanifest.ASRRuntimeSnapshot, error) {
	if settings == nil || !filepath.IsAbs(settings.AppDataRoot) {
		return localSenseVoiceProvider{}, nil, &ASRExecutionError{Code: "runtime_unavailable", Message: "Local model storage is unavailable."}
	}
	catalog, err := modelcatalog.Load()
	if err != nil {
		return localSenseVoiceProvider{}, nil, &ASRExecutionError{Code: "runtime_invalid", Message: "The local model catalog is invalid."}
	}
	model, err := catalog.Model(string(ASRModelSenseVoiceSmallINT8))
	if err != nil {
		return localSenseVoiceProvider{}, nil, &ASRExecutionError{Code: "model_unavailable", Message: "The selected local model is unavailable."}
	}
	modelOnlyStatus := modelcatalog.Status(settings.AppDataRoot, catalog.CatalogRevision, model)
	if modelOnlyStatus.State != "runtime_missing" {
		return localSenseVoiceProvider{}, nil, &ASRExecutionError{Code: modelOnlyStatus.State, Message: modelOnlyStatus.Message}
	}
	probe := localruntime.Probe(context.Background())
	runtimeReadiness := modelcatalog.RuntimeReadiness{RuntimeID: model.EngineRuntime, Version: probe.Version, State: modelcatalog.RuntimeMissing}
	if probe.State == localruntime.Invalid {
		runtimeReadiness.State = modelcatalog.RuntimeInvalid
	}
	if probe.State == localruntime.Ready {
		runtimeReadiness.State = modelcatalog.RuntimeReady
	}
	status := modelcatalog.StatusWithRuntime(settings.AppDataRoot, catalog.CatalogRevision, model, runtimeReadiness)
	if status.State != "ready" {
		return localSenseVoiceProvider{}, nil, &ASRExecutionError{Code: status.State, Message: status.Message}
	}
	files, err := modelcatalog.ResolveVerifiedModelFiles(settings.AppDataRoot, catalog.CatalogRevision, model)
	if err != nil {
		return localSenseVoiceProvider{}, nil, &ASRExecutionError{Code: "model_invalid", Message: "The installed local model failed verification."}
	}
	provider := localSenseVoiceProvider{runtimePath: probe.Path, modelPath: files.Model, tokensPath: files.Tokens}
	snapshot := &taskmanifest.ASRRuntimeSnapshot{
		ProviderID: string(ASRProviderLocalSenseVoice), ModelID: model.ModelID,
		ExecutionLocation: "local", RuntimeKind: "bundled_sidecar",
		RuntimeVersion: probe.Version, ModelVersion: model.ModelVersion,
		ModelSHA256: model.Archive.SHA256, Engine: "sensevoice", EngineRuntime: model.EngineRuntime,
	}
	return provider, snapshot, nil
}

func (provider localSenseVoiceProvider) Transcribe(ctx context.Context, request ASRRequest) (ASRResult, error) {
	fmt.Println("Status: Loading local model")
	audio, err := filepath.EvalSymlinks(request.AudioPath)
	if err != nil || !filepath.IsAbs(audio) {
		return ASRResult{}, &ASRExecutionError{Code: "invalid_audio", Message: "The local audio chunk is unavailable."}
	}
	info, err := os.Stat(audio)
	if err != nil || !info.Mode().IsRegular() || strings.ToLower(filepath.Ext(audio)) != ".wav" {
		return ASRResult{}, &ASRExecutionError{Code: "invalid_audio", Message: "Local speech recognition requires a normalized WAV chunk."}
	}
	requestIDBytes := make([]byte, 16)
	if _, err := rand.Read(requestIDBytes); err != nil {
		return ASRResult{}, &ASRExecutionError{Code: "runtime_failed", Message: "Local transcription could not be initialized."}
	}
	result, err := localruntime.Transcribe(ctx, provider.runtimePath, localruntime.Request{
		ProtocolVersion: localruntime.ProtocolVersion,
		RequestID:       hex.EncodeToString(requestIDBytes),
		AudioPath:       audio,
		ModelPath:       provider.modelPath,
		TokensPath:      provider.tokensPath,
		LanguageMode:    "chinese_first",
		UseITN:          true,
		Timestamps:      true,
	}, localruntime.PerChunkTimeout(info))
	if err != nil {
		var runtimeError *localruntime.Error
		if errors.As(err, &runtimeError) {
			return ASRResult{}, &ASRExecutionError{Code: runtimeError.Code, Message: runtimeError.Message}
		}
		return ASRResult{}, &ASRExecutionError{Code: "runtime_failed", Message: localruntime.SafeMessage(err)}
	}
	text := strings.TrimSpace(result.Text)
	if text == "" {
		return ASRResult{}, &ASRExecutionError{Code: "empty_transcript", Message: "The local speech runtime returned no transcript."}
	}
	return ASRResult{Text: text, Segments: result.Segments}, nil
}

func ensureASRReady(settings *Settings) (*taskmanifest.ASRRuntimeSnapshot, error) {
	if settings.ASRConfig().ProviderID != ASRProviderLocalSenseVoice {
		return nil, nil
	}
	_, snapshot, err := prepareLocalSenseVoice(settings)
	return snapshot, err
}

func localRuntimeStatus(root string) (modelcatalog.Readiness, error) {
	catalog, err := modelcatalog.Load()
	if err != nil {
		return modelcatalog.Readiness{}, err
	}
	model, err := catalog.Model(localruntime.ModelID)
	if err != nil {
		return modelcatalog.Readiness{}, err
	}
	modelOnlyStatus := modelcatalog.Status(root, catalog.CatalogRevision, model)
	if modelOnlyStatus.State != "runtime_missing" {
		return modelOnlyStatus, nil
	}
	probe := localruntime.Probe(context.Background())
	state := modelcatalog.RuntimeMissing
	if probe.State == localruntime.Invalid {
		state = modelcatalog.RuntimeInvalid
	} else if probe.State == localruntime.Ready {
		state = modelcatalog.RuntimeReady
	}
	return modelcatalog.StatusWithRuntime(root, catalog.CatalogRevision, model, modelcatalog.RuntimeReadiness{RuntimeID: model.EngineRuntime, Version: probe.Version, State: state}), nil
}

var _ ASRProvider = localSenseVoiceProvider{}
var _ = fmt.Sprintf
