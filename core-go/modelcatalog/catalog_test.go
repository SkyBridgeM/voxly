package modelcatalog

import (
	"strings"
	"testing"
)

func TestEmbeddedCatalogIdentity(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.Model("sensevoice-small-int8")
	if err != nil {
		t.Fatal(err)
	}
	if model.ModelVersion != "2024-07-17" || model.Archive.SizeBytes != 163002883 || model.Archive.SHA256 != "7d1efa2138a65b0b488df37f8b89e3d91a60676e416f515b952358d83dfd347e" {
		t.Fatalf("unexpected identity: %#v", model.Archive)
	}
	if strings.Contains(strings.ToLower(model.Source.URL), "latest") {
		t.Fatal("catalog URL is not pinned")
	}
	if model.Source.Kind != "github_release" || model.Source.Repository != "k2-fsa/sherpa-onnx" || model.Source.Revision != "asr-models" {
		t.Fatalf("unexpected source: %#v", model.Source)
	}
	if model.Source.URL != "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17.tar.bz2" {
		t.Fatalf("unexpected URL: %s", model.Source.URL)
	}
}

func TestEndpointNormalization(t *testing.T) {
	for _, test := range []struct {
		input, want string
		valid       bool
	}{
		{"", DefaultHuggingFaceEndpoint, true}, {"https://HF.Example/", "https://hf.example", true},
		{"http://hf.example", "", false}, {"https://user@hf.example", "", false}, {"https://hf.example/path", "", false},
		{"https://hf.example?q=1", "", false}, {"https://hf.example/#fragment", "", false},
	} {
		got, err := NormalizeHuggingFaceEndpoint(test.input)
		if test.valid && (err != nil || got != test.want) {
			t.Fatalf("Normalize(%q) = %q, %v", test.input, got, err)
		}
		if !test.valid && err == nil {
			t.Fatalf("Normalize(%q) accepted", test.input)
		}
	}
}

func TestHuggingFaceEndpointDoesNotRewriteGitHubAndPreservesCompatiblePath(t *testing.T) {
	github := "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/model.tar.bz2"
	if got, err := RewriteHuggingFaceOrigin(github, "https://hf.example"); err != nil || got != github {
		t.Fatalf("GitHub source changed: %q, %v", got, err)
	}
	source := "https://huggingface.co/fixed/repo/resolve/fixed-revision/model.bin"
	got, err := RewriteHuggingFaceOrigin(source, "https://HF.Example/")
	if err != nil || got != "https://hf.example/fixed/repo/resolve/fixed-revision/model.bin" {
		t.Fatalf("rewrite = %q, %v", got, err)
	}
}

func TestCatalogRejectsExcessivelyLongPath(t *testing.T) {
	catalog, _ := Load()
	catalog.Models[0].Archive.Entries[0].Path = strings.Repeat("a", maxRelativePathBytes+1)
	if err := catalog.Validate(); err == nil {
		t.Fatal("excessively long path accepted")
	}
}

func TestCatalogRejectsUnsafeFiles(t *testing.T) {
	catalog, _ := Load()
	catalog.Models[0].InstalledFiles[0].Path = "../escape"
	if err := catalog.Validate(); err == nil {
		t.Fatal("unsafe installed path accepted")
	}
}
