package taskmanifest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "docs", "fixtures", "task_manifest", name)
}

func TestLoadManifestFixtures(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		taskStatus string
		artifacts  map[string]string
	}{
		{
			name: "completed", fixture: "task_manifest_completed.json", taskStatus: "completed",
			artifacts: map[string]string{"transcript": "completed", "notes": "completed", "minutes": "completed"},
		},
		{
			name: "partial success", fixture: "task_manifest_partial_success.json", taskStatus: "completed_with_errors",
			artifacts: map[string]string{"transcript": "completed", "notes": "completed", "minutes": "failed"},
		},
		{
			name: "failed", fixture: "task_manifest_failed.json", taskStatus: "failed",
			artifacts: map[string]string{"transcript": "failed", "notes": "unavailable", "minutes": "unavailable"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := LoadManifest(fixturePath(t, test.fixture))
			if err != nil {
				t.Fatalf("LoadManifest() error = %v", err)
			}
			if manifest.Status != test.taskStatus {
				t.Fatalf("task status = %q, want %q", manifest.Status, test.taskStatus)
			}
			for name, want := range test.artifacts {
				artifact, ok := manifest.Artifacts.Get(name)
				if !ok {
					t.Fatalf("artifact %s missing", name)
				}
				if got := artifact.State; got != want {
					t.Errorf("artifact %s status = %q, want %q", name, got, want)
				}
			}
		})
	}
}

func TestInvalidManifestCases(t *testing.T) {
	base, err := os.ReadFile(fixturePath(t, "task_manifest_completed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]any
	if err := json.Unmarshal(base, &original); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		mutate  func(map[string]any)
		wantErr string
	}{
		{name: "missing schema version", mutate: func(value map[string]any) { delete(value, "schema_version") }, wantErr: "missing schema_version"},
		{name: "invalid artifact status", mutate: func(value map[string]any) {
			value["artifacts"].(map[string]any)["transcript"].(map[string]any)["status"] = "ready"
		}, wantErr: "invalid status"},
		{name: "malformed artifact error", mutate: func(value map[string]any) {
			artifact := value["artifacts"].(map[string]any)["minutes"].(map[string]any)
			artifact["status"] = "failed"
			delete(artifact, "path")
			artifact["error"] = map[string]any{"code": "failed", "message": "safe"}
		}, wantErr: "must contain code, message, and retryable"},
		{name: "unsupported schema version", mutate: func(value map[string]any) { value["schema_version"] = "2.0" }, wantErr: ErrUnsupportedSchemaVersion.Error()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := deepCopyMap(t, original)
			test.mutate(value)
			path := writeJSON(t, value)
			_, err := LoadManifest(path)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadManifest() error = %v, want containing %q", err, test.wantErr)
			}
			if test.name == "unsupported schema version" && !errors.Is(err, ErrUnsupportedSchemaVersion) {
				t.Fatalf("error = %v, want ErrUnsupportedSchemaVersion", err)
			}
		})
	}
}

func TestValidateManifestRequiresExecutionFields(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{name: "backend", field: "backend", wantErr: "missing execution.backend"},
		{name: "provider", field: "provider", wantErr: "missing execution.provider"},
		{name: "output mode", field: "output_mode", wantErr: "missing execution.output_mode"},
		{name: "status", field: "status", wantErr: "invalid execution.status"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(fixturePath(t, "task_manifest_completed.json"))
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			delete(value["execution"].(map[string]any), test.field)
			_, err = LoadManifest(writeJSON(t, value))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadManifest() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestWriteManifestRoundTripPreservesUnknownFields(t *testing.T) {
	data, err := os.ReadFile(fixturePath(t, "task_manifest_completed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	value["future_top_level"] = map[string]any{"enabled": true}
	value["execution"].(map[string]any)["future_execution"] = "preserve"
	value["artifacts"].(map[string]any)["transcript"].(map[string]any)["future_artifact"] = 7.0

	input := writeJSON(t, value)
	manifest, err := LoadManifest(input)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "manifest.json")
	if err := WriteManifest(output, manifest); err != nil {
		t.Fatalf("WriteManifest() error = %v", err)
	}
	reloaded, err := LoadManifest(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(reloaded.UnknownFields["future_top_level"]) == "" {
		t.Error("top-level unknown field was not preserved")
	}
	if string(reloaded.ExecutionUnknownFields["future_execution"]) == "" {
		t.Error("execution unknown field was not preserved")
	}
	transcript, _ := reloaded.Artifacts.Get("transcript")
	if string(transcript.UnknownFields["future_artifact"]) == "" {
		t.Error("artifact unknown field was not preserved")
	}
}

func TestValidationRejectsSensitiveUnknownFields(t *testing.T) {
	manifest, err := LoadManifest(fixturePath(t, "task_manifest_completed.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.UnknownFields["api_key"] = json.RawMessage(`"must-not-write"`)
	if err := manifest.WriteManifest(filepath.Join(t.TempDir(), "manifest.json")); err == nil || !strings.Contains(err.Error(), "forbidden sensitive field") {
		t.Fatalf("WriteManifest() error = %v, want forbidden sensitive field", err)
	}
}

func deepCopyMap(t *testing.T, source map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var copied map[string]any
	if err := json.Unmarshal(data, &copied); err != nil {
		t.Fatal(err)
	}
	return copied
}

func writeJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
