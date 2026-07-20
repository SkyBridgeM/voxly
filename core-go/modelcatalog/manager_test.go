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

var _ io.Reader
