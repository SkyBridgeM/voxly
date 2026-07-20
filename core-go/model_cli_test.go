package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yudi/meet-asr/core-go/modelcatalog"
)

func TestModelCatalogCLIProducesOneJSONDocument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runModelCommand("model-catalog", nil, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&stdout)
	var catalog modelcatalog.Catalog
	if err := decoder.Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if decoder.Decode(&struct{}{}) == nil {
		t.Fatal("more than one JSON document")
	}
	if stderr.Len() != 0 || len(catalog.Models) != 1 {
		t.Fatalf("stderr=%q catalog=%#v", stderr.String(), catalog)
	}
}

func TestModelStatusCLIDoesNotCreateStorage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer
	if err := runModelCommand("model-status", []string{"--app-data-root", root, "--model-id", "sensevoice-small-int8"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var status modelcatalog.Readiness
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.State != "not_installed" && status.State != "unsupported_platform" {
		t.Fatalf("state = %s", status.State)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("status created storage: %v", err)
	}
}

func TestNormalizeEndpointCLIRejectsUnsafeOriginWithEmptyStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runModelCommand("normalize-hf-endpoint", []string{"--endpoint", "http://unsafe.example"}, &stdout, &stderr); err == nil {
		t.Fatal("unsafe endpoint accepted")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
