package localruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	ProtocolVersion = "1.0"
	WrapperVersion  = "1.0.0"
	Engine          = "sherpa-onnx"
	EngineVersion   = "1.13.4"
	ModelID         = "sensevoice-small-int8"
	maxJSONBytes    = 4 << 20
)

type State string

const (
	Missing State = "missing"
	Invalid State = "invalid"
	Ready   State = "ready"
)

type Handshake struct {
	ProtocolVersion string   `json:"protocol_version"`
	WrapperVersion  string   `json:"wrapper_version"`
	Engine          string   `json:"engine"`
	EngineVersion   string   `json:"engine_version"`
	Platform        string   `json:"platform"`
	Architecture    string   `json:"architecture"`
	SupportedModels []string `json:"supported_model_ids"`
}

type Readiness struct {
	State   State
	Path    string
	Version string
	Message string
}

type Request struct {
	ProtocolVersion string `json:"protocol_version"`
	RequestID       string `json:"request_id"`
	AudioPath       string `json:"audio_path"`
	ModelPath       string `json:"model_path"`
	TokensPath      string `json:"tokens_path"`
	LanguageMode    string `json:"language_mode"`
	UseITN          bool   `json:"use_itn"`
	Timestamps      bool   `json:"timestamps"`
}

type Segment struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

type RuntimeIdentity struct {
	Engine         string `json:"engine"`
	EngineVersion  string `json:"version"`
	WrapperVersion string `json:"wrapper_version"`
	ModelID        string `json:"model_id"`
}

type Result struct {
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	Text            string          `json:"text"`
	Segments        []Segment       `json:"segments"`
	Runtime         RuntimeIdentity `json:"runtime"`
}

type Error struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func executableName() string {
	return executableNameFor(runtime.GOOS)
}

func executableNameFor(goos string) string {
	name := "voxly-sensevoice-wrapper"
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Resolve finds only the wrapper installed beside the trusted Go sidecar.
// Neither frontend arguments nor process environment can replace it.
func Resolve() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return verifiedExecutable(runtimeCandidatePath(executable, runtime.GOOS))
}

func runtimeCandidatePath(sidecarPath, goos string) string {
	return filepath.Join(filepath.Dir(sidecarPath), executableNameFor(goos))
}

func verifiedExecutable(path string) (string, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("runtime executable is unavailable")
	}
	return canonical, nil
}

func Probe(ctx context.Context) Readiness {
	path, err := Resolve()
	if err != nil {
		return Readiness{State: Missing, Message: "The bundled local speech runtime is unavailable."}
	}
	return probeExecutable(ctx, path)
}

func probeExecutable(ctx context.Context, path string) Readiness {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, path, "--version-json")
	configureCancellation(cmd)
	stdout, stderr, err := boundedOutput(cmd, 64<<10, 16<<10)
	_ = stderr
	if err != nil {
		return Readiness{State: Invalid, Path: path, Message: "The bundled local speech runtime failed validation."}
	}
	var handshake Handshake
	if decodeOne(stdout, &handshake) != nil || validateHandshake(handshake) != nil {
		return Readiness{State: Invalid, Path: path, Message: "The bundled local speech runtime returned an incompatible handshake."}
	}
	return Readiness{State: Ready, Path: path, Version: handshake.EngineVersion, Message: "The bundled local speech runtime is ready."}
}

func validateHandshake(value Handshake) error {
	if value.ProtocolVersion != ProtocolVersion || value.WrapperVersion != WrapperVersion || value.Engine != Engine || value.EngineVersion != EngineVersion || value.Platform != runtime.GOOS || value.Architecture != runtime.GOARCH {
		return errors.New("runtime identity mismatch")
	}
	for _, model := range value.SupportedModels {
		if model == ModelID {
			return nil
		}
	}
	return errors.New("runtime does not support the managed model")
}

func Transcribe(ctx context.Context, runtimePath string, request Request, timeout time.Duration) (Result, error) {
	if request.ProtocolVersion != ProtocolVersion || request.RequestID == "" || request.LanguageMode != "chinese_first" || !filepath.IsAbs(request.AudioPath) || !filepath.IsAbs(request.ModelPath) || !filepath.IsAbs(request.TokensPath) {
		return Result{}, &Error{Code: "invalid_request", Message: "The local transcription request is invalid."}
	}
	payload, err := json.Marshal(request)
	if err != nil || len(payload) > 64<<10 {
		return Result{}, &Error{Code: "invalid_request", Message: "The local transcription request is too large."}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, runtimePath)
	configureCancellation(cmd)
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	stdout, stderr, runErr := boundedOutput(cmd, maxJSONBytes, 64<<10)
	_ = stderr
	if runCtx.Err() != nil {
		code := "runtime_timeout"
		message := "Local transcription exceeded its time limit."
		if errors.Is(ctx.Err(), context.Canceled) {
			code, message = "cancelled", "Local transcription was cancelled."
		}
		return Result{}, &Error{Code: code, Message: message, Retryable: true}
	}
	if runErr != nil {
		return Result{}, &Error{Code: "runtime_failed", Message: "The local speech runtime could not transcribe this chunk.", Retryable: true}
	}
	var result Result
	if err := decodeOne(stdout, &result); err != nil {
		return Result{}, &Error{Code: "runtime_protocol_error", Message: "The local speech runtime returned an invalid result."}
	}
	if err := validateResult(request, result); err != nil {
		return Result{}, &Error{Code: "runtime_protocol_error", Message: "The local speech runtime returned an incompatible result."}
	}
	return result, nil
}

func validateResult(request Request, result Result) error {
	if result.ProtocolVersion != ProtocolVersion || result.RequestID != request.RequestID || result.Runtime.Engine != Engine || result.Runtime.EngineVersion != EngineVersion || result.Runtime.WrapperVersion != WrapperVersion || result.Runtime.ModelID != ModelID || len(result.Text) > 2<<20 || len(result.Segments) > 100000 {
		return errors.New("result identity or size mismatch")
	}
	var previous int64
	for _, segment := range result.Segments {
		if segment.StartMS < previous || segment.EndMS < segment.StartMS || segment.Text == "" || len(segment.Text) > 64<<10 {
			return errors.New("invalid segment sequence")
		}
		previous = segment.EndMS
	}
	return nil
}

func decodeOne(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("expected exactly one JSON document")
	}
	return nil
}

type limitBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (w *limitBuffer) Write(data []byte) (int, error) {
	if w.buffer.Len()+len(data) > w.limit {
		return 0, errors.New("process output exceeded limit")
	}
	return w.buffer.Write(data)
}

func boundedOutput(command *exec.Cmd, stdoutLimit, stderrLimit int) ([]byte, []byte, error) {
	stdout := &limitBuffer{limit: stdoutLimit}
	stderr := &limitBuffer{limit: stderrLimit}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	return stdout.buffer.Bytes(), stderr.buffer.Bytes(), err
}

func PerChunkTimeout(info os.FileInfo) time.Duration {
	const floor = 2 * time.Minute
	if info == nil || info.Size() <= 0 {
		return floor
	}
	// PCM16 mono 16 kHz is 32 kB/s. Allow 2x audio duration plus startup.
	estimated := time.Duration(info.Size()/32000) * time.Second
	timeout := 45*time.Second + 2*estimated
	if timeout < floor {
		return floor
	}
	if timeout > 45*time.Minute {
		return 45 * time.Minute
	}
	return timeout
}

func SafeMessage(err error) string {
	var runtimeError *Error
	if errors.As(err, &runtimeError) {
		return runtimeError.Message
	}
	return fmt.Sprintf("Local speech recognition failed.")
}
