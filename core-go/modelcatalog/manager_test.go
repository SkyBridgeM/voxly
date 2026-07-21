package modelcatalog

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tinyModel(payload []byte) Model {
	h := sha256.Sum256(payload)
	identity := FileIdentity{Path: "model.bin", SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(h[:])}
	return Model{ModelID: "fixture-model", ModelVersion: "1", Platforms: []string{CurrentPlatform()}, Archive: Archive{SizeBytes: int64(len(payload)), SHA256: identity.SHA256, RootDirectory: "fixture", Entries: []ArchiveEntry{{Path: identity.Path, Type: "file", SizeBytes: identity.SizeBytes, SHA256: identity.SHA256}}}, InstalledFiles: []FileIdentity{identity}}
}

func writeValidFixtureInstall(t *testing.T, root, revision string, model Model, payload []byte) Paths {
	t.Helper()
	paths, err := NewPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.prepare(); err != nil {
		t.Fatal(err)
	}
	directory := paths.modelDirectory(model)
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "model.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	receipt := Receipt{SchemaVersion: receiptSchema, ModelID: model.ModelID, ModelVersion: model.ModelVersion, CatalogRevision: revision, ArchiveSHA256: model.Archive.SHA256, InstalledFiles: model.InstalledFiles, InstalledAt: time.Now().UTC().Format(time.RFC3339), SourceKind: "manual_import"}
	if err := writeJSONAtomically(filepath.Join(directory, "receipt.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
	return paths
}

func tarBytes(t *testing.T, headers ...*tar.Header) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, header := range headers {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			_, _ = writer.Write(bytes.Repeat([]byte{'x'}, int(header.Size)))
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestExtractTarVerifiedAndReadiness(t *testing.T) {
	payload := []byte("model-data")
	model := tinyModel(payload)
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	_ = writer.WriteHeader(&tar.Header{Name: "fixture/", Typeflag: tar.TypeDir, Mode: 0700})
	_ = writer.WriteHeader(&tar.Header{Name: "fixture/model.bin", Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(payload))})
	_, _ = writer.Write(payload)
	_ = writer.Close()
	staging := t.TempDir()
	if err := extractTarVerified(context.Background(), tar.NewReader(bytes.NewReader(archive.Bytes())), staging, model, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(staging, "model.bin"))
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("installed data = %q, %v", data, err)
	}
}

func TestArchiveSafetyRejectsTraversalAndLinks(t *testing.T) {
	model := tinyModel([]byte("x"))
	for _, header := range []*tar.Header{
		{Name: "fixture/../model.bin", Typeflag: tar.TypeReg, Size: 1},
		{Name: "/fixture/model.bin", Typeflag: tar.TypeReg, Size: 1},
		{Name: "fixture/model.bin", Typeflag: tar.TypeSymlink, Linkname: "outside"},
		{Name: "fixture/model.bin", Typeflag: tar.TypeLink, Linkname: "outside"},
		{Name: "fixture/model.bin", Typeflag: tar.TypeChar},
	} {
		reader := tar.NewReader(bytes.NewReader(tarBytes(t, header)))
		if err := extractTarVerified(context.Background(), reader, t.TempDir(), model, nil); err == nil {
			t.Fatalf("unsafe header accepted: %#v", header)
		}
	}
}

func TestExtractionLimitsDuplicatesAndCaseCollisions(t *testing.T) {
	model := tinyModel([]byte("x"))
	tests := []struct {
		name    string
		model   Model
		headers []*tar.Header
		limits  ExtractionLimits
	}{
		{"too many entries", model, []*tar.Header{{Name: "fixture/model.bin", Typeflag: tar.TypeReg, Size: 1}}, ExtractionLimits{MaxEntries: 0, MaxTotal: 10, MaxFile: 10, MaxPathBytes: 512}},
		{"file too large", model, []*tar.Header{{Name: "fixture/model.bin", Typeflag: tar.TypeReg, Size: 1}}, ExtractionLimits{MaxEntries: 2, MaxTotal: 10, MaxFile: 0, MaxPathBytes: 512}},
		{"path too long", model, []*tar.Header{{Name: "fixture/model.bin", Typeflag: tar.TypeReg, Size: 1}}, ExtractionLimits{MaxEntries: 2, MaxTotal: 10, MaxFile: 10, MaxPathBytes: 5}},
		{"duplicate", model, []*tar.Header{{Name: "fixture/model.bin", Typeflag: tar.TypeReg, Size: 1}, {Name: "fixture/model.bin", Typeflag: tar.TypeReg, Size: 1}}, ExtractionLimits{MaxEntries: 3, MaxTotal: 10, MaxFile: 10, MaxPathBytes: 512}},
	}
	caseModel := tinyModel([]byte("x"))
	caseModel.Archive.Entries = append(caseModel.Archive.Entries, ArchiveEntry{Path: "MODEL.BIN", Type: "file", SizeBytes: 1, SHA256: caseModel.Archive.Entries[0].SHA256})
	tests = append(tests, struct {
		name    string
		model   Model
		headers []*tar.Header
		limits  ExtractionLimits
	}{"case collision", caseModel, []*tar.Header{{Name: "fixture/model.bin", Typeflag: tar.TypeReg, Size: 1}, {Name: "fixture/MODEL.BIN", Typeflag: tar.TypeReg, Size: 1}}, ExtractionLimits{MaxEntries: 3, MaxTotal: 10, MaxFile: 10, MaxPathBytes: 512}})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := tar.NewReader(bytes.NewReader(tarBytes(t, test.headers...)))
			if err := extractTarVerifiedWithLimits(context.Background(), reader, t.TempDir(), test.model, nil, test.limits); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}

func TestExtractedTotalLimitIsEnforcedBeforeSecondWrite(t *testing.T) {
	payload := []byte("x")
	model := tinyModel(payload)
	second := model.Archive.Entries[0]
	second.Path = "tokens.bin"
	model.Archive.Entries = append(model.Archive.Entries, second)
	headers := []*tar.Header{{Name: "fixture/model.bin", Typeflag: tar.TypeReg, Size: 1}, {Name: "fixture/tokens.bin", Typeflag: tar.TypeReg, Size: 1}}
	err := extractTarVerifiedWithLimits(context.Background(), tar.NewReader(bytes.NewReader(tarBytes(t, headers...))), t.TempDir(), model, nil, ExtractionLimits{MaxEntries: 3, MaxTotal: 1, MaxFile: 1, MaxPathBytes: 512})
	if err == nil {
		t.Fatal("extracted total limit was not enforced")
	}
}

func TestExtractionWriteFailureCannotCreateReceipt(t *testing.T) {
	model := tinyModel([]byte("x"))
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "model.bin"), []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	header := &tar.Header{Name: "fixture/model.bin", Typeflag: tar.TypeReg, Size: 1}
	if err := extractTarVerified(context.Background(), tar.NewReader(bytes.NewReader(tarBytes(t, header))), staging, model, nil); err == nil {
		t.Fatal("write collision accepted")
	}
	if _, err := os.Stat(filepath.Join(staging, "receipt.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed extraction created receipt: %v", err)
	}
}

func TestDownloadStreamRetryAndCleanup(t *testing.T) {
	payload := []byte("official archive fixture")
	model := tinyModel(payload)
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "download.partial")
	if err := downloadWithRetry(context.Background(), server.Client(), server.URL, destination, model.Archive, nil); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d", attempts)
	}
	data, _ := os.ReadFile(destination)
	if !bytes.Equal(data, payload) {
		t.Fatal("streamed payload mismatch")
	}
	model.Archive.SHA256 = strings.Repeat("0", 64)
	destination = filepath.Join(t.TempDir(), "bad.partial")
	if err := downloadWithRetry(context.Background(), server.Client(), server.URL, destination, model.Archive, nil); err == nil {
		t.Fatal("hash mismatch accepted")
	}
}

func TestDownloadCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for {
			_, err := w.Write([]byte("x"))
			if err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(time.Millisecond)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	expected := Archive{SizeBytes: 1024, SHA256: strings.Repeat("0", 64)}
	err := downloadWithRetry(ctx, server.Client(), server.URL, filepath.Join(t.TempDir(), "partial"), expected, nil)
	var managed *Error
	if !errors.As(err, &managed) || managed.Code != "download_cancelled" {
		t.Fatalf("error = %v", err)
	}
}

func TestManualImportRejectsWrongOrOversizedSourceAndCleansPartials(t *testing.T) {
	model := tinyModel([]byte("expected"))
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "archive.tar.bz2")
	if err := os.WriteFile(source, []byte("wrong---"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportArchive(context.Background(), root, "r", model, source, nil); err == nil {
		t.Fatal("wrong archive hash accepted")
	}
	paths, _ := NewPaths(root)
	entries, err := os.ReadDir(paths.Downloads)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partials remain: %v, %v", entries, err)
	}
	if _, err := os.Stat(paths.modelDirectory(model)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed import created final model: %v", err)
	}
	if err := os.WriteFile(source, []byte("expected-extra"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportArchive(context.Background(), root, "r", model, source, nil); err == nil {
		t.Fatal("oversized source accepted")
	}
	entries, _ = os.ReadDir(paths.Downloads)
	if len(entries) != 0 {
		t.Fatalf("oversized import left partials: %v", entries)
	}
}

func TestModelLockOwnershipAndStaleRecovery(t *testing.T) {
	paths, _ := NewPaths(t.TempDir())
	if err := paths.prepare(); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireModelLock(paths, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireModelLock(paths, "fixture"); err == nil {
		t.Fatal("active lock stolen")
	}
	replacement := lockOwner{Token: "replacement", PID: os.Getpid(), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeJSONAtomically(filepath.Join(lock.directory, "owner.json"), replacement, 0600); err != nil {
		t.Fatal(err)
	}
	lock.release()
	if _, err := os.Stat(lock.directory); err != nil {
		t.Fatal("unlock removed another owner")
	}
	_ = os.RemoveAll(lock.directory)
	directory := filepath.Join(paths.Locks, "fixture.lock")
	_ = os.Mkdir(directory, 0700)
	_ = os.WriteFile(filepath.Join(directory, "owner.json"), []byte(`{"token":"dead","pid":99999999,"created_at":"2000-01-01T00:00:00Z"}`), 0600)
	old := time.Now().Add(-time.Minute)
	_ = os.Chtimes(directory, old, old)
	recovered, err := acquireModelLock(paths, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	recovered.release()
}

func TestStoragePreparationRejectsSymlinkedManagedDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	paths, _ := NewPaths(root)
	if err := os.Symlink(outside, paths.Models); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := paths.prepare(); err == nil {
		t.Fatal("symlinked model directory accepted")
	}
}

func TestStatusDetectsCorruptionAndDeleteContainment(t *testing.T) {
	payload := []byte("model-data")
	model := tinyModel(payload)
	root := t.TempDir()
	paths, _ := NewPaths(root)
	_ = paths.prepare()
	directory := paths.modelDirectory(model)
	_ = os.MkdirAll(directory, 0700)
	_ = os.WriteFile(filepath.Join(directory, "model.bin"), payload, 0600)
	receipt := Receipt{SchemaVersion: receiptSchema, ModelID: model.ModelID, ModelVersion: model.ModelVersion, CatalogRevision: "r", ArchiveSHA256: model.Archive.SHA256, InstalledFiles: model.InstalledFiles, InstalledAt: time.Now().UTC().Format(time.RFC3339), SourceKind: "manual_import"}
	_ = writeJSONAtomically(filepath.Join(directory, "receipt.json"), receipt, 0600)
	if state := Status(root, "r", model).State; state != "runtime_missing" {
		t.Fatalf("state = %s", state)
	}
	_ = os.WriteFile(filepath.Join(directory, "model.bin"), []byte("corrupt!!!"), 0600)
	if state := Status(root, "r", model).State; state != "model_invalid" {
		t.Fatalf("state = %s", state)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	_ = os.Mkdir(outside, 0700)
	_ = os.RemoveAll(directory)
	_ = os.Symlink(outside, directory)
	if _, err := Delete(root, "r", model); err == nil {
		t.Fatal("symlink installation deleted")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside directory changed")
	}
}

func TestRuntimeReadinessComposesAfterModelVerification(t *testing.T) {
	payload := []byte("model-data")
	model := tinyModel(payload)
	model.EngineRuntime = "fixture-runtime"
	root := t.TempDir()
	writeValidFixtureInstall(t, root, "r", model, payload)
	missing := StatusWithRuntime(root, "r", model, RuntimeReadiness{RuntimeID: "fixture-runtime", State: RuntimeMissing})
	if missing.State != "runtime_missing" {
		t.Fatalf("missing state = %s", missing.State)
	}
	ready := StatusWithRuntime(root, "r", model, RuntimeReadiness{RuntimeID: "fixture-runtime", Version: "test-only", State: RuntimeReady})
	if ready.State != "ready" || ready.RuntimeVersion != "test-only" {
		t.Fatalf("ready = %#v", ready)
	}
	wrongRuntime := StatusWithRuntime(root, "r", model, RuntimeReadiness{RuntimeID: "other", State: RuntimeReady})
	if wrongRuntime.State != "runtime_missing" {
		t.Fatalf("wrong runtime state = %s", wrongRuntime.State)
	}
	_ = os.WriteFile(filepath.Join(root, "models", model.ModelID, model.ModelVersion, "model.bin"), []byte("corrupt!!!"), 0600)
	if state := StatusWithRuntime(root, "r", model, RuntimeReadiness{RuntimeID: "fixture-runtime", State: RuntimeReady}).State; state != "model_invalid" {
		t.Fatalf("runtime bypassed model validation: %s", state)
	}
}

func TestReceiptCannotOverrideCatalogIdentity(t *testing.T) {
	payload := []byte("model-data")
	model := tinyModel(payload)
	root := t.TempDir()
	paths := writeValidFixtureInstall(t, root, "r", model, payload)
	receiptPath := filepath.Join(paths.modelDirectory(model), "receipt.json")
	receipt, err := readReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	receipt.InstalledFiles[0].SHA256 = strings.Repeat("0", 64)
	if err := writeJSONAtomically(receiptPath, receipt, 0600); err != nil {
		t.Fatal(err)
	}
	if state := StatusWithRuntime(root, "r", model, RuntimeReadiness{RuntimeID: model.EngineRuntime, State: RuntimeReady}).State; state != "model_invalid" {
		t.Fatalf("receipt overrode catalog: %s", state)
	}
}

func TestDeleteIsContainedRepeatableAndPreservesUnrelatedData(t *testing.T) {
	payload := []byte("model-data")
	model := tinyModel(payload)
	root := t.TempDir()
	paths := writeValidFixtureInstall(t, root, "r", model, payload)
	unrelated := filepath.Join(root, "unrelated.txt")
	taskFixture := filepath.Join(root, "task-fixture", "task_manifest.json")
	_ = os.WriteFile(unrelated, []byte("keep"), 0600)
	_ = os.MkdirAll(filepath.Dir(taskFixture), 0700)
	_ = os.WriteFile(taskFixture, []byte("task-history"), 0600)
	result, err := Delete(root, "r", model)
	if err != nil || result.State != "not_installed" {
		t.Fatalf("delete = %#v, %v", result, err)
	}
	result, err = Delete(root, "r", model)
	if err != nil || result.State != "not_installed" {
		t.Fatalf("repeat delete = %#v, %v", result, err)
	}
	if data, _ := os.ReadFile(unrelated); string(data) != "keep" {
		t.Fatal("unrelated data changed")
	}
	if data, _ := os.ReadFile(taskFixture); string(data) != "task-history" {
		t.Fatal("Task fixture changed")
	}
	if _, err := os.Stat(paths.modelDirectory(model)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("model directory remains: %v", err)
	}
}

func TestDeleteRejectsBusyInstallAndSymlinkedModelParent(t *testing.T) {
	model := tinyModel([]byte("x"))
	root := t.TempDir()
	paths, _ := NewPaths(root)
	_ = paths.prepare()
	lock, err := acquireModelLock(paths, model.ModelID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Delete(root, "r", model); err == nil {
		t.Fatal("delete ignored active install lock")
	}
	lock.release()
	outside := t.TempDir()
	parent := filepath.Join(paths.Models, model.ModelID)
	_ = os.Symlink(outside, parent)
	if _, err := Delete(root, "r", model); err == nil {
		t.Fatal("symlinked model parent accepted")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside directory changed")
	}
}

var _ io.Reader
