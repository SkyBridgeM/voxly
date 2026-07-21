package localruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type windowsAssetManifest struct {
	SchemaVersion string `json:"schema_version"`
	Target        string `json:"target"`
	Engine        struct {
		Version     string `json:"version"`
		Revision    string `json:"revision"`
		ONNXRuntime string `json:"onnxruntime_version"`
		ONNXAsset   string `json:"onnxruntime_asset"`
		ONNXSHA256  string `json:"onnxruntime_sha256"`
		Linkage     string `json:"linkage"`
		CPUBaseline string `json:"cpu_baseline"`
	} `json:"engine"`
	Files []struct {
		Path              string `json:"path"`
		BundleDestination string `json:"bundle_destination"`
	} `json:"files"`
	NonSystemDLLs []string `json:"non_system_dlls"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func TestWindowsRuntimeAssetManifestIsExactAndPortable(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "runtime", "sensevoice", "windows-runtime-assets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest windowsAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != "1.0" || manifest.Target != "x86_64-pc-windows-msvc" || manifest.Engine.Version != EngineVersion || manifest.Engine.Revision != "142807252687d81b40d6315f23470a1512a00de3" || manifest.Engine.ONNXRuntime != "1.27.0" || manifest.Engine.ONNXAsset != "onnxruntime-win-x64-static_lib-MT-Release-1.27.0.tar.bz2" || manifest.Engine.ONNXSHA256 != "6db0cdd3bcb208911758b7336820d19ecff0359a7003e0ec65895be701741288" || manifest.Engine.Linkage != "static" {
		t.Fatalf("unexpected Windows runtime identity: %#v", manifest)
	}
	baseline := strings.ToLower(manifest.Engine.CPUBaseline)
	if !strings.Contains(baseline, "sse2") || !strings.Contains(baseline, "/arch:avx2") || len(manifest.NonSystemDLLs) != 0 {
		t.Fatalf("unexpected CPU/DLL contract: %#v", manifest.Engine)
	}
	want := []string{"licenses/LICENSE.sherpa-onnx", "licenses/THIRD_PARTY_NOTICES.md", "runtime-manifest.json", "voxly-sensevoice-wrapper.exe"}
	got := make([]string, 0, len(manifest.Files))
	seen := map[string]bool{}
	for _, file := range manifest.Files {
		if filepath.IsAbs(file.Path) || filepath.IsAbs(file.BundleDestination) || strings.Contains(file.Path, "..") || seen[strings.ToLower(file.Path)] {
			t.Fatalf("unsafe or duplicate asset: %#v", file)
		}
		seen[strings.ToLower(file.Path)] = true
		got = append(got, file.Path)
	}
	sort.Strings(got)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("asset paths = %v, want %v", got, want)
	}
}

func TestWindowsPreparationHasNoNativeCPUFlagsLatestOrModelWeights(t *testing.T) {
	root := repositoryRoot(t)
	cmake, err := os.ReadFile(filepath.Join(root, "runtime", "sensevoice", "CMakeLists.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(cmake))
	for _, forbidden := range []string{"/arch:avx", "/arch:avx2", "-march=native", "refs/tags/latest"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("forbidden build setting %q", forbidden)
		}
	}
	for _, directory := range []string{filepath.Join(root, "runtime"), filepath.Join(root, "scripts", "windows")} {
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".onnx" || ext == ".bin" || ext == ".tar" || ext == ".bz2" {
				t.Fatalf("model or archive binary committed at %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWindowsScriptsExposeSingleSupportedCommands(t *testing.T) {
	root := repositoryRoot(t)
	required := map[string]string{
		"check-sensevoice-environment.ps1":   "codex/integrate-sensevoice-runtime",
		"build-sensevoice-runtime.ps1":       "Visual Studio 17 2022",
		"smoke-sensevoice-handshake.ps1":     "--version-json",
		"smoke-sensevoice-runtime.ps1":       "Parameter(Mandatory)",
		"smoke-sensevoice-packaged.ps1":      "Parameter(Mandatory)",
		"collect-sensevoice-diagnostics.ps1": "processes",
	}
	for name, marker := range required {
		data, err := os.ReadFile(filepath.Join(root, "scripts", "windows", name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		lower := strings.ToLower(text)
		if !strings.Contains(text, marker) || strings.Contains(lower, "invoke-expression") || strings.Contains(text, "/Users/") || strings.Contains(text, `C:\src`) {
			t.Fatalf("unsafe or incomplete Windows script %s", name)
		}
	}
	common, err := os.ReadFile(filepath.Join(root, "scripts", "windows", "sensevoice-common.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(common), "ArgumentList.Add") || !strings.Contains(string(common), "Kill($true)") || !strings.Contains(string(common), "ReadToEndAsync") {
		t.Fatal("Windows process helper lost typed arguments, process-tree timeout, or concurrent pipe draining")
	}
	build, err := os.ReadFile(filepath.Join(root, "scripts", "windows", "build-sensevoice-runtime.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(build)), "copy-item *dll") {
		t.Fatal("Windows build must not wildcard-copy DLLs")
	}
}

func TestTauriDeclaresTargetNeutralSidecarBasesAndRuntimeResources(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "desktop", "src-tauri", "tauri.conf.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Bundle struct {
			ExternalBin []string `json:"externalBin"`
			Resources   []string `json:"resources"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if strings.Join(config.Bundle.ExternalBin, "|") != "binaries/meeting-asr-go|binaries/voxly-sensevoice-wrapper" || strings.Join(config.Bundle.Resources, "|") != "resources/sensevoice/*" {
		t.Fatalf("unexpected Tauri runtime packaging: %#v", config.Bundle)
	}
}
