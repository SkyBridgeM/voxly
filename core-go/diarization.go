package main

import "context"

// SpeakerDiarizer is a reserved processing boundary, not an enabled provider.
// No implementation is registered or called by the current transcription flow.
// A future implementation must use ctx for cancellation and must not mutate or
// delete caller-owned audio. Clustering is scoped to one recording.
type SpeakerDiarizer interface {
	Diarize(ctx context.Context, request DiarizationRequest) (DiarizationResult, error)
}

// DiarizationRequest uses a normalized mono PCM16 WAV whose sample zero is the
// original recording's sample zero. SpeechRegions are VAD hints, not assertions
// that each region contains a single speaker. Ranges are half-open [start, end)
// in SampleRate units, never offsets inferred from chunk numbers.
type DiarizationRequest struct {
	AudioPath     string
	SampleRate    int
	TotalSamples  int64
	SpeechRegions []SpeechRegion
}

type SpeechRegion struct {
	StartSample int64
	EndSample   int64
}

// SpeakerTurn uses the request's original-audio timeline. SpeakerID is an
// anonymous, recording-local label, not a person's name. Turns need not align
// with VAD or ASR chunks; multiple speakers may occur within a single chunk.
// The representation permits overlap without promising a model can resolve it.
type SpeakerTurn struct {
	StartSample int64
	EndSample   int64
	SpeakerID   string
}

// DiarizationResult retains model and clustering identity for future provenance.
// Persistence, transcript alignment and UI are deliberately not wired yet.
type DiarizationResult struct {
	ModelID     string
	ModelSHA256 string
	Parameters  string
	Turns       []SpeakerTurn
}
