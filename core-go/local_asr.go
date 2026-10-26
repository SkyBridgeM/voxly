package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yudi/meet-asr/core-go/modelcatalog"
)

func LocalSenseVoiceCapabilities() ASRProviderCapabilities {
	return ASRProviderCapabilities{
		ContractVersion: asrCapabilityContractVersion,
		ProviderID:      string(ASRProviderLocalSenseVoice), ModelID: string(ASRModelSenseVoiceSmallINT8),
		AcceptedSourceFormats:  []string{"m4a", "mp3", "wav"},
		AlwaysRequiresChunking: true, RecommendedChunkMS: 60_000,
		MaximumConcurrency: 1, RecommendedConcurrency: 1,
	}
}

func localModelRoot(root string) (string, error) {
	if root == "" {
		root = modelcatalog.DefaultRoot()
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("local model storage must be an absolute path")
	}
	return root, nil
}

func localASRStatus(root string) (modelcatalog.Readiness, error) {
	root, err := localModelRoot(root)
	if err != nil {
		return modelcatalog.Readiness{}, err
	}
	catalog, err := modelcatalog.Load()
	if err != nil {
		return modelcatalog.Readiness{}, err
	}
	model, err := catalog.Model(string(ASRModelSenseVoiceSmallINT8))
	if err != nil {
		return modelcatalog.Readiness{}, err
	}
	status := modelcatalog.Status(root, catalog.CatalogRevision, model)
	if status.State != "ready" {
		return status, &ASRExecutionError{Code: status.State, Message: status.Message}
	}
	return status, nil
}

func normalizeLocalASRLanguage(language string) (string, error) {
	if language == "" {
		return "zh", nil
	}
	if language != "zh" && language != "auto" {
		return "", fmt.Errorf("local ASR language must be zh or auto")
	}
	return language, nil
}

type localASRProvider struct{ root, language string }

func (provider localASRProvider) Transcribe(ctx context.Context, request ASRRequest) (ASRResult, error) {
	if err := ctx.Err(); err != nil {
		return ASRResult{}, err
	}
	root, err := localModelRoot(provider.root)
	if err != nil {
		return ASRResult{}, err
	}
	catalog, err := modelcatalog.Load()
	if err != nil {
		return ASRResult{}, err
	}
	model, err := catalog.Model(string(ASRModelSenseVoiceSmallINT8))
	if err != nil {
		return ASRResult{}, err
	}
	directory, release, status, err := modelcatalog.OpenInstalled(root, catalog.CatalogRevision, model)
	if err != nil {
		var managed *modelcatalog.Error
		if errors.As(err, &managed) {
			return ASRResult{}, &ASRExecutionError{Code: managed.Code, Message: managed.Message}
		}
		return ASRResult{}, err
	}
	defer release()
	wav := request.AudioPath
	if !request.PCM16 {
		// Conversion output stays beside the Task's audio chunk, never in model
		// storage. Both subprocesses are killed by cancellation; no cloud fallback.
		temporary, err := os.MkdirTemp(filepath.Dir(request.AudioPath), ".sensevoice-")
		if err != nil {
			return ASRResult{}, err
		}
		defer os.RemoveAll(temporary)
		wav = filepath.Join(temporary, "audio.wav")
		conversion := exec.CommandContext(ctx, ffmpegBinary(), "-nostdin", "-v", "error", "-i", request.AudioPath, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-y", wav)
		if err := conversion.Run(); err != nil {
			if ctx.Err() != nil {
				return ASRResult{}, ctx.Err()
			}
			return ASRResult{}, fmt.Errorf("local audio conversion failed: %w", err)
		}
	}
	language, err := normalizeLocalASRLanguage(provider.language)
	if err != nil {
		return ASRResult{}, err
	}
	command := exec.CommandContext(ctx, modelcatalog.RuntimeBinary(), filepath.Join(directory, "model.int8.onnx"), filepath.Join(directory, "tokens.txt"), wav, "--language", language)
	// The native runtime has no need for cloud credentials.
	command.Env = []string{}
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ASRResult{}, ctx.Err()
		}
		return ASRResult{}, fmt.Errorf("local SenseVoice inference failed: %w", err)
	}
	var result struct {
		Text *string `json:"text"`
	}
	if json.Unmarshal(stdout.Bytes(), &result) != nil || result.Text == nil {
		return ASRResult{}, fmt.Errorf("local SenseVoice returned invalid JSON")
	}
	return ASRResult{Language: language, Text: strings.TrimSpace(*result.Text), RuntimeID: status.RuntimeID, RuntimeVersion: status.RuntimeVersion, ModelVersion: model.ModelVersion}, nil
}
