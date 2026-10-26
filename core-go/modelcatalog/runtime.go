package modelcatalog

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const SenseVoiceRuntimeVersion = "1.13.8"

// RuntimeBinary is always a sibling of the Go sidecar. Never discover native
// executables through PATH, the model archive, or environment overrides.
func RuntimeBinary() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	name := "voxly-sensevoice-wrapper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(executable), name)
}

func ProbeRuntime(model Model) RuntimeReadiness {
	result := MissingRuntime(model)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, RuntimeBinary(), "--version").Output()
	if err != nil {
		return result
	}
	var identity struct {
		Protocol  int    `json:"protocol"`
		RuntimeID string `json:"runtime_id"`
		Version   string `json:"version"`
	}
	if json.Unmarshal(output, &identity) != nil || identity.Protocol != 1 || identity.RuntimeID != model.EngineRuntime || identity.Version != SenseVoiceRuntimeVersion {
		return result
	}
	return RuntimeReadiness{RuntimeID: identity.RuntimeID, Version: identity.Version, State: RuntimeReady}
}

func DefaultRoot() string {
	root, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(root, "com.skybridgem.voxly")
}

// OpenInstalled holds the same cross-process lock as installation and deletion
// until inference ends, and verifies the catalog/receipt before exposing paths.
func OpenInstalled(root, revision string, model Model) (string, func(), Readiness, error) {
	paths, err := NewPaths(root)
	if err != nil {
		return "", nil, Readiness{}, err
	}
	if err := paths.prepare(); err != nil {
		return "", nil, Readiness{}, err
	}
	lock, err := acquireModelLock(paths, model.ModelID)
	if err != nil {
		return "", nil, Readiness{}, err
	}
	status := Status(root, revision, model)
	if status.State != "ready" {
		lock.release()
		return "", nil, status, modelError(status.State, status.Message, false)
	}
	return paths.modelDirectory(model), lock.release, status, nil
}
