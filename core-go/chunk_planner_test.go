package main

import (
	"reflect"
	"testing"
)

func duration(value int64) *int64 { return &value }

func mimoPlannerInput(durationMS *int64) ChunkPlannerInput {
	return ChunkPlannerInput{
		Audio:                    AudioMetadata{DurationMS: durationMS, SizeBytes: 1024, Format: "m4a"},
		Capabilities:             MimoCapabilities("mimo-v2.5-asr"),
		Strategy:                 ChunkStrategyProviderAuto,
		RequestedChunkDurationMS: 300_000,
		MaximumChunkBytes:        10 * 1024 * 1024,
		RequestedConcurrency:     1,
	}
}

func TestPlanChunksCharacterizesCurrentMimoBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		durationMS int64
		want       []ChunkBoundary
	}{
		{"short audio", 30_000, []ChunkBoundary{{Index: 1, StartMS: 0, EndMS: 30_000}}},
		{"exact chunk limit", 300_000, []ChunkBoundary{{Index: 1, StartMS: 0, EndMS: 300_000}}},
		{"just above chunk limit", 300_001, []ChunkBoundary{
			{Index: 1, StartMS: 0, EndMS: 300_000},
			{Index: 2, StartMS: 300_000, EndMS: 300_001},
		}},
		{"multiple chunks and final partial", 750_000, []ChunkBoundary{
			{Index: 1, StartMS: 0, EndMS: 300_000},
			{Index: 2, StartMS: 300_000, EndMS: 600_000},
			{Index: 3, StartMS: 600_000, EndMS: 750_000},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := PlanChunks(mimoPlannerInput(duration(test.durationMS)))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plan.Boundaries, test.want) {
				t.Fatalf("boundaries mismatch\ngot:  %+v\nwant: %+v", plan.Boundaries, test.want)
			}
			if plan.OverlapMS != 0 || plan.EffectiveConcurrency != 1 {
				t.Fatalf("current Mimo overlap/concurrency changed: %+v", plan)
			}
		})
	}
}

func TestPlanChunksCurrentMimoRetryDurations(t *testing.T) {
	plan, err := PlanChunks(mimoPlannerInput(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{300_000, 150_000, 75_000, 60_000}
	if !reflect.DeepEqual(plan.RetryChunkDurationsMS, want) {
		t.Fatalf("retry durations mismatch: got %v want %v", plan.RetryChunkDurationsMS, want)
	}
	if plan.Boundaries != nil {
		t.Fatalf("runtime without duration inspection must not fabricate boundaries: %+v", plan.Boundaries)
	}
	if plan.RequestedChunkDurationMS != 300_000 || plan.EffectiveChunkDurationMS != 300_000 {
		t.Fatalf("chunk duration changed: %+v", plan)
	}
}

func TestPlanChunksDeterministic(t *testing.T) {
	input := mimoPlannerInput(duration(750_000))
	first, err := PlanChunks(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanChunks(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same normalized inputs produced different plans\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestPlanChunksRejectsInvalidMetadataAndLimits(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ChunkPlannerInput)
	}{
		{"zero duration", func(i *ChunkPlannerInput) { i.Audio.DurationMS = duration(0) }},
		{"negative duration", func(i *ChunkPlannerInput) { i.Audio.DurationMS = duration(-1) }},
		{"negative size", func(i *ChunkPlannerInput) { i.Audio.SizeBytes = -1 }},
		{"unsupported format", func(i *ChunkPlannerInput) { i.Audio.Format = "flac" }},
		{"invalid upload limit", func(i *ChunkPlannerInput) { i.MaximumChunkBytes = 0 }},
		{"invalid strategy", func(i *ChunkPlannerInput) { i.Strategy = ChunkStrategyCustom }},
		{"invalid concurrency", func(i *ChunkPlannerInput) { i.RequestedConcurrency = 2 }},
		{"provider duration hard limit", func(i *ChunkPlannerInput) {
			i.Capabilities.MaximumSourceDurationMS = 10_000
			i.Audio.DurationMS = duration(10_001)
		}},
		{"provider size hard limit", func(i *ChunkPlannerInput) {
			i.Capabilities.MaximumSourceFileBytes = 100
			i.Audio.SizeBytes = 101
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := mimoPlannerInput(duration(30_000))
			test.mutate(&input)
			if _, err := PlanChunks(input); err == nil {
				t.Fatal("expected planner validation error")
			}
		})
	}
}

func TestPlanChunksClampsExistingSubMinuteOverride(t *testing.T) {
	input := mimoPlannerInput(duration(90_000))
	input.RequestedChunkDurationMS = 30_000
	plan, err := PlanChunks(input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.EffectiveChunkDurationMS != 60_000 {
		t.Fatalf("existing 60-second minimum changed: %+v", plan)
	}
	want := []ChunkBoundary{
		{Index: 1, StartMS: 0, EndMS: 60_000},
		{Index: 2, StartMS: 60_000, EndMS: 90_000},
	}
	if !reflect.DeepEqual(plan.Boundaries, want) {
		t.Fatalf("clamped boundaries mismatch: got %+v want %+v", plan.Boundaries, want)
	}
}

func TestPlanChunksRejectsImpossibleBoundaryCount(t *testing.T) {
	const maxInt64 = int64(^uint64(0) >> 1)
	input := mimoPlannerInput(duration(maxInt64))
	if _, err := PlanChunks(input); err == nil {
		t.Fatal("expected planner safety-limit error")
	}
}
