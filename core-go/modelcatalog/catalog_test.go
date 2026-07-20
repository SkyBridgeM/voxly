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

func TestCatalogRejectsUnsafeFiles(t *testing.T) {
	catalog, _ := Load()
	catalog.Models[0].InstalledFiles[0].Path = "../escape"
	if err := catalog.Validate(); err == nil {
		t.Fatal("unsafe installed path accepted")
	}
}
