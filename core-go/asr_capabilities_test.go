package main

import "testing"

func TestMimoCapabilitiesSnapshot(t *testing.T) {
	capabilities := MimoCapabilities("mimo-v2.5-asr")
	if err := capabilities.Validate(); err != nil {
		t.Fatal(err)
	}
	if capabilities.ContractVersion != "1" || capabilities.ProviderID != "mimo" || capabilities.ModelID != "mimo-v2.5-asr" {
		t.Fatalf("unexpected Mimo identity: %+v", capabilities)
	}
	if capabilities.RecommendedChunkMS != 300_000 || capabilities.RecommendedOverlapMS != 0 {
		t.Fatalf("unexpected Mimo chunk defaults: %+v", capabilities)
	}
	if capabilities.MaximumConcurrency != 1 || capabilities.RecommendedConcurrency != 1 {
		t.Fatalf("unexpected Mimo concurrency: %+v", capabilities)
	}
	if capabilities.MaximumSourceDurationMS != 0 || capabilities.MaximumSourceFileBytes != 0 || capabilities.MaximumChunkMS != 0 {
		t.Fatal("unverified Mimo hard limits must remain explicitly unknown")
	}
}

func TestASRCapabilitiesValidation(t *testing.T) {
	valid := MimoCapabilities("model")
	tests := []struct {
		name   string
		mutate func(*ASRProviderCapabilities)
	}{
		{"negative source duration", func(c *ASRProviderCapabilities) { c.MaximumSourceDurationMS = -1 }},
		{"negative source size", func(c *ASRProviderCapabilities) { c.MaximumSourceFileBytes = -1 }},
		{"zero recommended duration", func(c *ASRProviderCapabilities) { c.RecommendedChunkMS = 0 }},
		{"recommended above maximum", func(c *ASRProviderCapabilities) { c.MaximumChunkMS = c.RecommendedChunkMS - 1 }},
		{"negative overlap", func(c *ASRProviderCapabilities) { c.RecommendedOverlapMS = -1 }},
		{"overlap equals duration", func(c *ASRProviderCapabilities) { c.RecommendedOverlapMS = c.RecommendedChunkMS }},
		{"zero concurrency", func(c *ASRProviderCapabilities) { c.MaximumConcurrency = 0 }},
		{"recommended concurrency above maximum", func(c *ASRProviderCapabilities) { c.RecommendedConcurrency = c.MaximumConcurrency + 1 }},
		{"inconsistent unchunked capability", func(c *ASRProviderCapabilities) { c.AlwaysRequiresChunking = true }},
		{"unsupported empty formats", func(c *ASRProviderCapabilities) { c.AcceptedSourceFormats = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capabilities := valid
			capabilities.AcceptedSourceFormats = append([]string(nil), valid.AcceptedSourceFormats...)
			test.mutate(&capabilities)
			if err := capabilities.Validate(); err == nil {
				t.Fatal("expected invalid capability snapshot")
			}
		})
	}
}
