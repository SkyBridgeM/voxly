package localruntime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestTargetExecutableNamesAndUnicodeRequestSerialization(t *testing.T) {
	if got := executableNameFor("windows"); got != "voxly-sensevoice-wrapper.exe" {
		t.Fatalf("windows executable = %q", got)
	}
	if got := executableNameFor("darwin"); got != "voxly-sensevoice-wrapper" {
		t.Fatalf("darwin executable = %q", got)
	}
	windowsSidecar := filepath.Join("C:\\", "Users", "Test User", "Desktop", "语音 测试", "meeting-asr-go.exe")
	if got := runtimeCandidatePath(windowsSidecar, "windows"); filepath.Base(got) != "voxly-sensevoice-wrapper.exe" || filepath.Dir(got) != filepath.Dir(windowsSidecar) {
		t.Fatalf("windows runtime candidate = %q", got)
	}
	request := Request{ProtocolVersion: ProtocolVersion, RequestID: "windows-unicode", AudioPath: `C:\Users\Test User\Desktop\语音 测试\会议录音 01.wav`, ModelPath: `C:\Users\Test User\AppData\Local\Voxly\模型\model.int8.onnx`, TokensPath: `C:\Users\Test User\AppData\Local\Voxly\模型\tokens.txt`, LanguageMode: "chinese_first"}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil || decoded != request {
		t.Fatalf("unicode request round trip = %#v, %v", decoded, err)
	}
}

func fakeRuntime(t *testing.T, mode string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fake runtime is Unix-only")
	}
	path := filepath.Join(t.TempDir(), "fake-runtime")
	script := `#!/bin/sh
if [ "$1" = "--version-json" ]; then
  printf '%s\n' '{"protocol_version":"1.0","wrapper_version":"1.0.0","engine":"sherpa-onnx","engine_version":"1.13.4","platform":"` + runtime.GOOS + `","architecture":"` + runtime.GOARCH + `","supported_model_ids":["sensevoice-small-int8"]}'
  exit 0
fi
case "` + mode + `" in
  malformed) printf 'not-json\n' ;;
  oversized) head -c 5000000 /dev/zero ;;
  timeout) sleep 5 ;;
  mismatch) printf '%s\n' '{"protocol_version":"1.0","request_id":"wrong","text":"x","segments":[],"runtime":{"engine":"sherpa-onnx","version":"1.13.4","wrapper_version":"1.0.0","model_id":"sensevoice-small-int8"}}' ;;
  *) sed -n 's/.*"request_id":"\([^"]*\)".*/{"protocol_version":"1.0","request_id":"\1","text":"你好 Voxly。","segments":[],"runtime":{"engine":"sherpa-onnx","version":"1.13.4","wrapper_version":"1.0.0","model_id":"sensevoice-small-int8"}}/p' ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProbeValidAndMissing(t *testing.T) {
	path := fakeRuntime(t, "valid")
	if output, err := exec.Command(path, "--version-json").CombinedOutput(); err != nil {
		t.Fatalf("fake handshake failed: %v: %s", err, output)
	}
	status := probeExecutable(context.Background(), path)
	if status.State != Ready || status.Version != EngineVersion {
		t.Fatalf("status = %#v", status)
	}
	if _, err := verifiedExecutable(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing executable accepted")
	}
	if status := probeExecutable(context.Background(), filepath.Join(t.TempDir(), "missing")); status.State != Invalid {
		t.Fatalf("missing status = %#v", status)
	}
}

func TestTranscribeProtocolValidation(t *testing.T) {
	request := Request{ProtocolVersion: ProtocolVersion, RequestID: "request-1", AudioPath: "/tmp/audio.wav", ModelPath: "/tmp/model.onnx", TokensPath: "/tmp/tokens.txt", LanguageMode: "chinese_first", UseITN: true, Timestamps: true}
	for _, mode := range []string{"malformed", "oversized", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := Transcribe(context.Background(), fakeRuntime(t, mode), request, time.Second); err == nil {
				t.Fatal("invalid runtime result accepted")
			}
		})
	}
	result, err := Transcribe(context.Background(), fakeRuntime(t, "valid"), request, time.Second)
	if err != nil || result.Text != "你好 Voxly。" {
		t.Fatalf("result = %#v, %v", result, err)
	}
}

func TestTranscribeCancellationAndTimeout(t *testing.T) {
	request := Request{ProtocolVersion: ProtocolVersion, RequestID: "request-1", AudioPath: "/tmp/audio.wav", ModelPath: "/tmp/model.onnx", TokensPath: "/tmp/tokens.txt", LanguageMode: "chinese_first"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Transcribe(ctx, fakeRuntime(t, "timeout"), request, time.Second); err == nil {
		t.Fatal("cancelled runtime succeeded")
	}
	if _, err := Transcribe(context.Background(), fakeRuntime(t, "timeout"), request, 20*time.Millisecond); err == nil {
		t.Fatal("timed-out runtime succeeded")
	}
}

func TestDecodeRejectsUnknownAndMultipleDocuments(t *testing.T) {
	var handshake Handshake
	data, _ := json.Marshal(map[string]any{"protocol_version": "1.0", "unknown": true})
	if decodeOne(data, &handshake) == nil {
		t.Fatal("unknown handshake field accepted")
	}
	if decodeOne([]byte(`{} {}`), &handshake) == nil {
		t.Fatal("multiple JSON documents accepted")
	}
}

func TestHandshakeRejectsWrongIdentityAndModel(t *testing.T) {
	valid := Handshake{ProtocolVersion: ProtocolVersion, WrapperVersion: WrapperVersion, Engine: Engine, EngineVersion: EngineVersion, Platform: runtime.GOOS, Architecture: runtime.GOARCH, SupportedModels: []string{ModelID}}
	if err := validateHandshake(valid); err != nil {
		t.Fatal(err)
	}
	tests := []Handshake{valid, valid, valid, valid}
	tests[0].Engine = "wrong"
	tests[1].Platform = "wrong"
	tests[2].Architecture = "wrong"
	tests[3].SupportedModels = []string{"other"}
	for _, value := range tests {
		if validateHandshake(value) == nil {
			t.Fatalf("invalid handshake accepted: %#v", value)
		}
	}
}

func TestResultRejectsInvalidSegmentsAndOversizedText(t *testing.T) {
	request := Request{ProtocolVersion: ProtocolVersion, RequestID: "request"}
	base := Result{ProtocolVersion: ProtocolVersion, RequestID: request.RequestID, Text: "ok", Runtime: RuntimeIdentity{Engine: Engine, EngineVersion: EngineVersion, WrapperVersion: WrapperVersion, ModelID: ModelID}}
	invalid := base
	invalid.Segments = []Segment{{StartMS: 10, EndMS: 20, Text: "one"}, {StartMS: 5, EndMS: 30, Text: "two"}}
	if validateResult(request, invalid) == nil {
		t.Fatal("out-of-order segments accepted")
	}
	invalid = base
	invalid.Text = string(make([]byte, (2<<20)+1))
	if validateResult(request, invalid) == nil {
		t.Fatal("oversized text accepted")
	}
}
