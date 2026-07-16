package main

import (
	"fmt"
	"strings"
)

const chunkPlannerContractVersion = "1"
const maxPlannedChunkBoundaries = 1_000_000

type ChunkStrategy string

const (
	ChunkStrategyProviderAuto ChunkStrategy = "provider_auto"
	ChunkStrategyUnchunked    ChunkStrategy = "unchunked"
	ChunkStrategyCustom       ChunkStrategy = "custom"
)

type AudioMetadata struct {
	// DurationMS is nil in the current runtime because it does not perform an
	// extra duration probe. Tests and future inspectors may provide it.
	DurationMS *int64
	SizeBytes  int64
	Format     string
}

type ChunkPlannerInput struct {
	Audio                    AudioMetadata
	Capabilities             ASRProviderCapabilities
	Strategy                 ChunkStrategy
	RequestedChunkDurationMS int64
	MaximumChunkBytes        int64
	RequestedConcurrency     int
}

type ChunkBoundary struct {
	Index   int
	StartMS int64
	EndMS   int64
}

type ChunkPlan struct {
	PlannerVersion           string
	CapabilityVersion        string
	ProviderID               string
	ModelID                  string
	Strategy                 ChunkStrategy
	RequestedChunkDurationMS int64
	EffectiveChunkDurationMS int64
	OverlapMS                int64
	EffectiveConcurrency     int
	MaximumChunkBytes        int64
	RetryChunkDurationsMS    []int64
	Boundaries               []ChunkBoundary
	Reason                   string
}

// PlanChunks is pure and deterministic. Current production calls use
// provider_auto; unused custom execution remains deliberately deferred.
func PlanChunks(input ChunkPlannerInput) (ChunkPlan, error) {
	if err := input.Capabilities.Validate(); err != nil {
		return ChunkPlan{}, fmt.Errorf("invalid ASR capabilities: %w", err)
	}
	if input.Audio.SizeBytes < 0 {
		return ChunkPlan{}, fmt.Errorf("audio size cannot be negative")
	}
	format := strings.TrimPrefix(strings.ToLower(input.Audio.Format), ".")
	if !input.Capabilities.acceptsSourceFormat(format) {
		return ChunkPlan{}, fmt.Errorf("ASR provider %s does not accept source format %q", input.Capabilities.ProviderID, format)
	}
	if input.Audio.DurationMS != nil && *input.Audio.DurationMS <= 0 {
		return ChunkPlan{}, fmt.Errorf("audio duration must be positive when provided")
	}
	if input.Capabilities.MaximumSourceDurationMS > 0 && input.Audio.DurationMS != nil && *input.Audio.DurationMS > input.Capabilities.MaximumSourceDurationMS {
		return ChunkPlan{}, fmt.Errorf("audio duration exceeds the ASR provider hard limit")
	}
	if input.Capabilities.MaximumSourceFileBytes > 0 && input.Audio.SizeBytes > input.Capabilities.MaximumSourceFileBytes {
		return ChunkPlan{}, fmt.Errorf("audio size exceeds the ASR provider hard limit")
	}
	if input.MaximumChunkBytes <= 0 {
		return ChunkPlan{}, fmt.Errorf("maximum chunk upload size must be positive")
	}

	if input.Strategy == "" {
		input.Strategy = ChunkStrategyProviderAuto
	}
	if input.Strategy != ChunkStrategyProviderAuto {
		return ChunkPlan{}, fmt.Errorf("chunk strategy %q is not implemented by the current Mimo runtime", input.Strategy)
	}

	effectiveDuration := input.RequestedChunkDurationMS
	if effectiveDuration == 0 {
		effectiveDuration = input.Capabilities.RecommendedChunkMS
	}
	minimumDuration := int64(minChunkSeconds) * 1000
	if effectiveDuration < minimumDuration {
		effectiveDuration = minimumDuration
	}
	if input.Capabilities.MaximumChunkMS > 0 && effectiveDuration > input.Capabilities.MaximumChunkMS {
		return ChunkPlan{}, fmt.Errorf("requested chunk duration exceeds the ASR provider hard limit")
	}

	concurrency := input.RequestedConcurrency
	if concurrency == 0 {
		concurrency = input.Capabilities.RecommendedConcurrency
	}
	if concurrency <= 0 || concurrency > input.Capabilities.MaximumConcurrency {
		return ChunkPlan{}, fmt.Errorf("requested concurrency exceeds the ASR provider hard limit")
	}

	retryDurations, err := retryChunkDurationsMS(effectiveDuration, minimumDuration)
	if err != nil {
		return ChunkPlan{}, err
	}
	boundaries, err := plannedBoundaries(input.Audio.DurationMS, effectiveDuration)
	if err != nil {
		return ChunkPlan{}, err
	}

	return ChunkPlan{
		PlannerVersion:           chunkPlannerContractVersion,
		CapabilityVersion:        input.Capabilities.ContractVersion,
		ProviderID:               input.Capabilities.ProviderID,
		ModelID:                  input.Capabilities.ModelID,
		Strategy:                 input.Strategy,
		RequestedChunkDurationMS: input.RequestedChunkDurationMS,
		EffectiveChunkDurationMS: effectiveDuration,
		OverlapMS:                input.Capabilities.RecommendedOverlapMS,
		EffectiveConcurrency:     concurrency,
		MaximumChunkBytes:        input.MaximumChunkBytes,
		RetryChunkDurationsMS:    retryDurations,
		Boundaries:               boundaries,
		Reason:                   "current Mimo runtime always segments input using provider defaults or the existing chunk-duration override",
	}, nil
}

func retryChunkDurationsMS(initial, minimum int64) ([]int64, error) {
	if initial <= 0 || minimum <= 0 {
		return nil, fmt.Errorf("chunk durations must be positive")
	}
	values := []int64{initial}
	current := initial
	for current > minimum {
		next := current / 2
		if next < minimum {
			next = minimum
		}
		if next == values[len(values)-1] {
			break
		}
		values = append(values, next)
		current = next
	}
	return values, nil
}

func plannedBoundaries(durationMS *int64, chunkDurationMS int64) ([]ChunkBoundary, error) {
	if durationMS == nil {
		return nil, nil
	}
	if *durationMS <= 0 || chunkDurationMS <= 0 {
		return nil, fmt.Errorf("duration and chunk duration must be positive")
	}
	count64 := int64(1) + (*durationMS-1)/chunkDurationMS
	if count64 > maxPlannedChunkBoundaries {
		return nil, fmt.Errorf("chunk boundary count exceeds the planner safety limit")
	}
	boundaries := make([]ChunkBoundary, 0, int(count64))
	for start, index := int64(0), 1; start < *durationMS; index++ {
		end := *durationMS
		if chunkDurationMS <= *durationMS-start {
			end = start + chunkDurationMS
		}
		if end > *durationMS {
			end = *durationMS
		}
		boundaries = append(boundaries, ChunkBoundary{Index: index, StartMS: start, EndMS: end})
		start = end
	}
	return boundaries, nil
}
