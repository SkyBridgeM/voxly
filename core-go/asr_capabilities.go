package main

import (
	"fmt"
	"slices"
	"strings"
)

const asrCapabilityContractVersion = "1"

// ASRProviderCapabilities describes provider constraints and defaults needed
// by the current runtime. Zero hard-limit values mean the provider limit is
// not known; they are not permission to invent or bypass a provider limit.
type ASRProviderCapabilities struct {
	ContractVersion         string
	ProviderID              string
	ModelID                 string
	MaximumSourceDurationMS int64
	MaximumSourceFileBytes  int64
	AcceptedSourceFormats   []string
	PreferredChunkFormat    string
	SupportsUnchunkedInput  bool
	AlwaysRequiresChunking  bool
	RecommendedChunkMS      int64
	MaximumChunkMS          int64
	RecommendedOverlapMS    int64
	MaximumConcurrency      int
	RecommendedConcurrency  int
}

// MimoCapabilities returns the explicit capability snapshot for the current
// Mimo runtime. Mimo duration limits are not encoded because the current
// implementation does not have a verified provider duration limit.
func MimoCapabilities(model string) ASRProviderCapabilities {
	return ASRProviderCapabilities{
		ContractVersion:        asrCapabilityContractVersion,
		ProviderID:             "mimo",
		ModelID:                model,
		AcceptedSourceFormats:  []string{"m4a", "mp3", "wav"},
		PreferredChunkFormat:   "source",
		SupportsUnchunkedInput: true,
		RecommendedChunkMS:     int64(defaultChunkMinutes) * 60 * 1000,
		RecommendedOverlapMS:   0,
		MaximumConcurrency:     1,
		RecommendedConcurrency: 1,
	}
}

func SenseVoiceCapabilities(model string) ASRProviderCapabilities {
	return ASRProviderCapabilities{
		ContractVersion: asrCapabilityContractVersion, ProviderID: string(ASRProviderLocalSenseVoice), ModelID: model,
		AcceptedSourceFormats: []string{"m4a", "mp3", "wav"}, PreferredChunkFormat: "wav",
		SupportsUnchunkedInput: true, RecommendedChunkMS: int64(defaultChunkMinutes) * 60 * 1000,
		MaximumChunkMS: 30 * 60 * 1000, MaximumConcurrency: 1, RecommendedConcurrency: 1,
	}
}

func (capabilities ASRProviderCapabilities) Validate() error {
	if capabilities.ContractVersion == "" {
		return fmt.Errorf("ASR capability contract version is required")
	}
	if capabilities.ProviderID == "" {
		return fmt.Errorf("ASR provider ID is required")
	}
	if capabilities.ModelID == "" {
		return fmt.Errorf("ASR model ID is required")
	}
	if capabilities.MaximumSourceDurationMS < 0 || capabilities.MaximumSourceFileBytes < 0 {
		return fmt.Errorf("ASR source hard limits cannot be negative")
	}
	if len(capabilities.AcceptedSourceFormats) == 0 {
		return fmt.Errorf("ASR accepted source formats are required")
	}
	if capabilities.PreferredChunkFormat != "source" && capabilities.PreferredChunkFormat != "wav" {
		return fmt.Errorf("invalid ASR preferred chunk format")
	}
	if capabilities.AlwaysRequiresChunking && capabilities.SupportsUnchunkedInput {
		return fmt.Errorf("ASR capabilities cannot both require chunking and support unchunked input")
	}
	for _, format := range capabilities.AcceptedSourceFormats {
		if strings.TrimSpace(format) == "" || format != strings.ToLower(format) {
			return fmt.Errorf("invalid ASR source format %q", format)
		}
	}
	if capabilities.RecommendedChunkMS <= 0 {
		return fmt.Errorf("ASR recommended chunk duration must be positive")
	}
	if capabilities.MaximumChunkMS < 0 {
		return fmt.Errorf("ASR maximum chunk duration cannot be negative")
	}
	if capabilities.MaximumChunkMS > 0 && capabilities.RecommendedChunkMS > capabilities.MaximumChunkMS {
		return fmt.Errorf("ASR recommended chunk duration exceeds the hard maximum")
	}
	if capabilities.RecommendedOverlapMS < 0 || capabilities.RecommendedOverlapMS >= capabilities.RecommendedChunkMS {
		return fmt.Errorf("ASR recommended overlap must be non-negative and shorter than a chunk")
	}
	if capabilities.MaximumConcurrency <= 0 || capabilities.RecommendedConcurrency <= 0 {
		return fmt.Errorf("ASR concurrency must be positive")
	}
	if capabilities.RecommendedConcurrency > capabilities.MaximumConcurrency {
		return fmt.Errorf("ASR recommended concurrency exceeds the hard maximum")
	}
	return nil
}

func (capabilities ASRProviderCapabilities) acceptsSourceFormat(format string) bool {
	return slices.Contains(capabilities.AcceptedSourceFormats, strings.ToLower(format))
}
