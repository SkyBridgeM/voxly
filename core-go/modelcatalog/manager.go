package modelcatalog

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	receiptSchema        = "1.0"
	installedIndexSchema = "1.0"
	maxArchiveEntries    = 32
	maxExtractedBytes    = int64(260 * 1024 * 1024)
	maxRedirects         = 4
)

type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Detail    string `json:"detail,omitempty"`
}

func (err *Error) Error() string { return err.Code + ": " + err.Message }

func modelError(code, message string, retryable bool) error {
	return &Error{Code: code, Message: message, Retryable: retryable}
}

type Paths struct {
	Root      string
	Models    string
	Downloads string
	Staging   string
	Locks     string
	Catalog   string
}

func NewPaths(root string) (Paths, error) {
	abs, err := filepath.Abs(root)
	if err != nil || strings.TrimSpace(root) == "" {
		return Paths{}, modelError("storage_unavailable", "Local model storage is unavailable.", false)
	}
	return Paths{
		Root: abs, Models: filepath.Join(abs, "models"), Downloads: filepath.Join(abs, "downloads"),
		Staging: filepath.Join(abs, "staging"), Locks: filepath.Join(abs, "locks"), Catalog: filepath.Join(abs, "model-catalog"),
	}, nil
}

func (paths Paths) prepare() error {
	for _, directory := range []string{paths.Root, paths.Models, paths.Downloads, paths.Staging, paths.Locks, paths.Catalog} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			return modelError("storage_unavailable", "Local model storage could not be prepared.", true)
		}
		info, err := os.Lstat(directory)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return modelError("storage_unavailable", "Local model storage contains an unsafe directory.", false)
		}
		_ = os.Chmod(directory, 0700)
	}
	return nil
}

func (paths Paths) modelDirectory(model Model) string {
	return filepath.Join(paths.Models, model.ModelID, model.ModelVersion)
}

type Receipt struct {
	SchemaVersion   string         `json:"schema_version"`
	ModelID         string         `json:"model_id"`
	ModelVersion    string         `json:"model_version"`
	CatalogRevision string         `json:"catalog_revision"`
	ArchiveSHA256   string         `json:"archive_sha256"`
	InstalledFiles  []FileIdentity `json:"installed_files"`
	InstalledAt     string         `json:"installed_at"`
	SourceKind      string         `json:"source_kind"`
}

type Readiness struct {
	ModelID       string `json:"model_id"`
	ModelVersion  string `json:"model_version"`
	State         string `json:"state"`
	Message       string `json:"message"`
	InstalledSize int64  `json:"installed_size_bytes"`
}

func Status(root, catalogRevision string, model Model) Readiness {
	status := Readiness{ModelID: model.ModelID, ModelVersion: model.ModelVersion, State: "not_installed", Message: "Model is not installed."}
	if !SupportsCurrentPlatform(model) {
		status.State, status.Message = "unsupported_platform", "This model is not supported on this platform."
		return status
	}
	paths, err := NewPaths(root)
	if err != nil {
		status.State, status.Message = "storage_unavailable", "Local model storage is unavailable."
		return status
	}
	directory := paths.modelDirectory(model)
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return status
	}
	if err != nil {
		status.State, status.Message = "storage_unavailable", "Installed model storage cannot be inspected."
		return status
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		status.State, status.Message = "model_invalid", "Installed model data is invalid."
		return status
	}
	receipt, err := readReceipt(filepath.Join(directory, "receipt.json"))
	if err != nil || receipt.SchemaVersion != receiptSchema || receipt.ModelID != model.ModelID || receipt.ModelVersion != model.ModelVersion || receipt.CatalogRevision != catalogRevision || receipt.ArchiveSHA256 != model.Archive.SHA256 {
		status.State, status.Message = "model_invalid", "Installed model receipt is invalid."
		return status
	}
	if !sameFileIdentities(receipt.InstalledFiles, model.InstalledFiles) {
		status.State, status.Message = "model_invalid", "Installed model receipt does not match the catalog."
		return status
	}
	for _, expected := range model.InstalledFiles {
		candidate := filepath.Join(directory, filepath.FromSlash(expected.Path))
		actual, err := verifyRegularFile(candidate, expected.SizeBytes, expected.SHA256)
		if err != nil {
			status.State, status.Message = "model_invalid", "Installed model files failed verification."
			return status
		}
		status.InstalledSize += actual
	}
	status.State = "runtime_missing"
	status.Message = "Model installed; the local speech runtime will arrive in the next implementation stage."
	return status
}

func sameFileIdentities(left, right []FileIdentity) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func readReceipt(path string) (Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

type ProgressFunc func(state string, completed, total int64)

func DefaultHTTPClient(timeout time.Duration) *http.Client {
	client := &http.Client{Timeout: timeout}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return modelError("redirect_rejected", "The model source redirected too many times.", false)
		}
		if err := validateDownloadURL(request.URL); err != nil {
			return err
		}
		return nil
	}
	return client
}

func DownloadAndInstall(ctx context.Context, root, catalogRevision string, model Model, client *http.Client, progress ProgressFunc) (Readiness, error) {
	if !SupportsCurrentPlatform(model) {
		return Readiness{}, modelError("unsupported_platform", "This model is not supported on this platform.", false)
	}
	paths, err := NewPaths(root)
	if err != nil {
		return Readiness{}, err
	}
	if err := paths.prepare(); err != nil {
		return Readiness{}, err
	}
	lock, err := acquireModelLock(paths, model.ModelID)
	if err != nil {
		return Readiness{}, err
	}
	defer lock.release()
	if current := Status(root, catalogRevision, model); current.State == "runtime_missing" || current.State == "ready" {
		return current, nil
	}
	u, err := url.Parse(model.Source.URL)
	if err != nil || validateDownloadURL(u) != nil {
		return Readiness{}, modelError("source_rejected", "The official model source is invalid.", false)
	}
	partial := filepath.Join(paths.Downloads, model.ModelID+"."+randomToken()+".partial")
	defer os.Remove(partial)
	if progress != nil {
		progress("downloading", 0, model.Archive.SizeBytes)
	}
	if err := downloadWithRetry(ctx, client, u.String(), partial, model.Archive, progress); err != nil {
		return Readiness{}, err
	}
	return installArchiveLocked(ctx, paths, catalogRevision, model, partial, "official_download", progress)
}

func downloadWithRetry(ctx context.Context, client *http.Client, sourceURL, destination string, expected Archive, progress ProgressFunc) error {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		_ = os.Remove(destination)
		last = downloadFile(ctx, client, sourceURL, destination, expected, progress)
		if last == nil {
			return nil
		}
		var managed *Error
		if !errors.As(last, &managed) || !managed.Retryable || errors.Is(ctx.Err(), context.Canceled) {
			return last
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return modelError("download_cancelled", "Model download was cancelled.", true)
			case <-timer.C:
			}
		}
	}
	return last
}

func ImportArchive(ctx context.Context, root, catalogRevision string, model Model, sourcePath string, progress ProgressFunc) (Readiness, error) {
	paths, err := NewPaths(root)
	if err != nil {
		return Readiness{}, err
	}
	if err := paths.prepare(); err != nil {
		return Readiness{}, err
	}
	lock, err := acquireModelLock(paths, model.ModelID)
	if err != nil {
		return Readiness{}, err
	}
	defer lock.release()
	if current := Status(root, catalogRevision, model); current.State == "runtime_missing" || current.State == "ready" {
		return current, nil
	}
	info, err := os.Lstat(sourcePath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Readiness{}, modelError("source_rejected", "Select the exact official model archive.", false)
	}
	partial := filepath.Join(paths.Downloads, model.ModelID+"."+randomToken()+".partial")
	defer os.Remove(partial)
	if progress != nil {
		progress("verifying", 0, model.Archive.SizeBytes)
	}
	if err := copyAndVerify(ctx, sourcePath, partial, model.Archive, progress); err != nil {
		return Readiness{}, err
	}
	return installArchiveLocked(ctx, paths, catalogRevision, model, partial, "manual_import", progress)
}

func downloadFile(ctx context.Context, client *http.Client, sourceURL, destination string, expected Archive, progress ProgressFunc) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return modelError("source_rejected", "The official model source is invalid.", false)
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return modelError("download_cancelled", "Model download was cancelled.", true)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return modelError("download_timeout", "Model download timed out.", true)
		}
		return modelError("network_unavailable", "Could not reach the official model source.", true)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return modelError("network_unavailable", "The official model source returned an error.", response.StatusCode >= 500)
	}
	if response.ContentLength > expected.SizeBytes || response.ContentLength > maxExtractedBytes {
		return modelError("size_mismatch", "The downloaded model size did not match the catalog.", false)
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return modelError("storage_unavailable", "The model download could not be saved.", true)
	}
	hash := sha256.New()
	written, copyErr := copyWithProgress(ctx, io.MultiWriter(file, hash), io.LimitReader(response.Body, expected.SizeBytes+1), expected.SizeBytes, progress, "downloading")
	syncErr, closeErr := file.Sync(), file.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil || closeErr != nil {
		return modelError("storage_unavailable", "The model download could not be committed.", true)
	}
	if written != expected.SizeBytes {
		return modelError("size_mismatch", "The downloaded model size did not match the catalog.", false)
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return modelError("hash_mismatch", "The downloaded model failed integrity verification.", false)
	}
	return nil
}

func copyAndVerify(ctx context.Context, source, destination string, expected Archive, progress ProgressFunc) error {
	input, err := os.Open(source)
	if err != nil {
		return modelError("source_rejected", "The selected model archive could not be read.", false)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return modelError("storage_unavailable", "The imported model could not be staged.", true)
	}
	hash := sha256.New()
	written, copyErr := copyWithProgress(ctx, io.MultiWriter(output, hash), io.LimitReader(input, expected.SizeBytes+1), expected.SizeBytes, progress, "verifying")
	syncErr, closeErr := output.Sync(), output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil || closeErr != nil {
		return modelError("storage_unavailable", "The imported model could not be committed.", true)
	}
	if written != expected.SizeBytes {
		return modelError("size_mismatch", "The selected archive size did not match the catalog.", false)
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return modelError("hash_mismatch", "The selected archive failed integrity verification.", false)
	}
	return nil
}

func copyWithProgress(ctx context.Context, destination io.Writer, source io.Reader, total int64, progress ProgressFunc, state string) (int64, error) {
	buffer := make([]byte, 64*1024)
	var written int64
	var lastReported int64
	for {
		if err := ctx.Err(); err != nil {
			return written, modelError("download_cancelled", "Model operation was cancelled.", true)
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			n, writeErr := destination.Write(buffer[:count])
			written += int64(n)
			if writeErr != nil || n != count {
				return written, modelError("storage_unavailable", "Model data could not be written.", true)
			}
			if progress != nil && (written-lastReported >= 1024*1024 || written == total) {
				progress(state, written, total)
				lastReported = written
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, modelError("network_unavailable", "Model data could not be read.", true)
		}
	}
}

func installArchiveLocked(ctx context.Context, paths Paths, catalogRevision string, model Model, archivePath, sourceKind string, progress ProgressFunc) (Readiness, error) {
	if progress != nil {
		progress("installing", 0, int64(len(model.Archive.Entries)))
	}
	staging := filepath.Join(paths.Staging, model.ModelID+"."+randomToken())
	if err := os.Mkdir(staging, 0700); err != nil {
		return Readiness{}, modelError("storage_unavailable", "Model staging could not be created.", true)
	}
	defer os.RemoveAll(staging)
	if err := extractVerified(ctx, archivePath, staging, model, progress); err != nil {
		return Readiness{}, err
	}
	receipt := Receipt{SchemaVersion: receiptSchema, ModelID: model.ModelID, ModelVersion: model.ModelVersion, CatalogRevision: catalogRevision, ArchiveSHA256: model.Archive.SHA256, InstalledFiles: model.InstalledFiles, InstalledAt: time.Now().UTC().Format(time.RFC3339), SourceKind: sourceKind}
	if err := writeJSONAtomically(filepath.Join(staging, "receipt.json"), receipt, 0600); err != nil {
		return Readiness{}, modelError("storage_unavailable", "Model receipt could not be saved.", true)
	}
	final := paths.modelDirectory(model)
	if err := os.MkdirAll(filepath.Dir(final), 0700); err != nil {
		return Readiness{}, modelError("storage_unavailable", "Model directory could not be prepared.", true)
	}
	if _, err := os.Lstat(final); err == nil {
		return Readiness{}, modelError("model_invalid", "An invalid installation already occupies the model directory.", false)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Readiness{}, modelError("storage_unavailable", "Model directory could not be inspected.", true)
	}
	if err := os.Rename(staging, final); err != nil {
		return Readiness{}, modelError("storage_unavailable", "Model installation could not be committed.", true)
	}
	if err := writeInstalledIndex(paths, catalogRevision, model); err != nil {
		_ = os.Rename(final, staging)
		return Readiness{}, err
	}
	return Status(paths.Root, catalogRevision, model), nil
}

func extractVerified(ctx context.Context, archivePath, staging string, model Model, progress ProgressFunc) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return modelError("archive_invalid", "The model archive could not be opened.", false)
	}
	defer file.Close()
	return extractTarVerified(ctx, tar.NewReader(bzip2.NewReader(file)), staging, model, progress)
}

func extractTarVerified(ctx context.Context, reader *tar.Reader, staging string, model Model, progress ProgressFunc) error {
	expected := map[string]ArchiveEntry{}
	installed := map[string]FileIdentity{}
	for _, entry := range model.Archive.Entries {
		expected[entry.Path] = entry
	}
	for _, entry := range model.InstalledFiles {
		installed[entry.Path] = entry
	}
	seen, seenFold := map[string]bool{}, map[string]bool{}
	var totalSize int64
	entryCount := 0
	for {
		if ctx.Err() != nil {
			return modelError("download_cancelled", "Model installation was cancelled.", true)
		}
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return modelError("archive_invalid", "The model archive is malformed.", false)
		}
		entryCount++
		if entryCount > maxArchiveEntries {
			return modelError("archive_unsafe", "The model archive contains too many entries.", false)
		}
		name := strings.TrimSuffix(filepath.ToSlash(header.Name), "/")
		if name == model.Archive.RootDirectory && header.Typeflag == tar.TypeDir {
			continue
		}
		prefix := model.Archive.RootDirectory + "/"
		if !strings.HasPrefix(name, prefix) {
			return modelError("archive_unsafe", "The model archive has an unexpected root directory.", false)
		}
		relative := strings.TrimPrefix(name, prefix)
		if !safeRelative(relative) || seen[relative] || seenFold[strings.ToLower(relative)] {
			return modelError("archive_unsafe", "The model archive contains an unsafe or duplicate path.", false)
		}
		seen[relative], seenFold[strings.ToLower(relative)] = true, true
		identity, ok := expected[relative]
		if !ok {
			return modelError("archive_unsafe", "The model archive contains an unexpected entry.", false)
		}
		if identity.Type == "directory" {
			if header.Typeflag != tar.TypeDir {
				return modelError("archive_unsafe", "The model archive entry type is unsafe.", false)
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return modelError("archive_unsafe", "The model archive contains a link or special file.", false)
		}
		if header.Size != identity.SizeBytes || header.Size < 0 || header.Size > maxExtractedBytes {
			return modelError("size_mismatch", "A model archive entry has an unexpected size.", false)
		}
		totalSize += header.Size
		if totalSize > maxExtractedBytes {
			return modelError("archive_unsafe", "The model archive expands beyond the safe limit.", false)
		}
		hash := sha256.New()
		var destination io.Writer = hash
		var output *os.File
		if _, keep := installed[relative]; keep {
			target := filepath.Join(staging, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return modelError("storage_unavailable", "Model staging could not be prepared.", true)
			}
			var openErr error
			output, openErr = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if openErr != nil {
				return modelError("storage_unavailable", "Model file could not be staged.", true)
			}
			destination = io.MultiWriter(hash, output)
		}
		written, copyErr := io.CopyN(destination, reader, header.Size)
		if output != nil {
			syncErr, closeErr := output.Sync(), output.Close()
			if syncErr != nil || closeErr != nil {
				return modelError("storage_unavailable", "Model file could not be committed.", true)
			}
		}
		if copyErr != nil || written != header.Size {
			return modelError("archive_invalid", "The model archive ended unexpectedly.", false)
		}
		if hex.EncodeToString(hash.Sum(nil)) != identity.SHA256 {
			return modelError("installed_file_invalid", "A model archive file failed integrity verification.", false)
		}
		if progress != nil {
			progress("installing", int64(len(seen)), int64(len(expected)))
		}
	}
	if len(seen) != len(expected) {
		return modelError("installed_file_missing", "The model archive is missing required files.", false)
	}
	return nil
}

func Delete(root, catalogRevision string, model Model) (Readiness, error) {
	paths, err := NewPaths(root)
	if err != nil {
		return Readiness{}, err
	}
	if err := paths.prepare(); err != nil {
		return Readiness{}, err
	}
	lock, err := acquireModelLock(paths, model.ModelID)
	if err != nil {
		return Readiness{}, err
	}
	defer lock.release()
	final := paths.modelDirectory(model)
	info, err := os.Lstat(final)
	if errors.Is(err, os.ErrNotExist) {
		return Status(root, catalogRevision, model), nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Readiness{}, modelError("model_invalid", "Installed model data is unsafe to delete.", false)
	}
	quarantine := filepath.Join(paths.Staging, model.ModelID+".delete."+randomToken())
	if err := os.Rename(final, quarantine); err != nil {
		return Readiness{}, modelError("permission_denied", "Installed model data could not be isolated for deletion.", true)
	}
	if err := os.RemoveAll(quarantine); err != nil {
		_ = os.Rename(quarantine, final)
		return Readiness{}, modelError("permission_denied", "Installed model data could not be deleted.", true)
	}
	if err := writeInstalledIndex(paths, catalogRevision, Model{}); err != nil {
		return Readiness{}, err
	}
	return Status(root, catalogRevision, model), nil
}

type InstalledIndex struct {
	SchemaVersion   string               `json:"schema_version"`
	CatalogRevision string               `json:"catalog_revision"`
	Models          []InstalledIndexItem `json:"models"`
}

type InstalledIndexItem struct {
	ModelID      string `json:"model_id"`
	ModelVersion string `json:"model_version"`
}

func writeInstalledIndex(paths Paths, revision string, model Model) error {
	index := InstalledIndex{SchemaVersion: installedIndexSchema, CatalogRevision: revision, Models: []InstalledIndexItem{}}
	if model.ModelID != "" {
		index.Models = append(index.Models, InstalledIndexItem{ModelID: model.ModelID, ModelVersion: model.ModelVersion})
	}
	if err := writeJSONAtomically(filepath.Join(paths.Catalog, "installed.json"), index, 0600); err != nil {
		return modelError("storage_unavailable", "Installed model index could not be saved.", true)
	}
	return nil
}

func writeJSONAtomically(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + "." + randomToken() + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		backup := path + "." + randomToken() + ".bak"
		hadOriginal := false
		if _, statErr := os.Lstat(path); statErr == nil {
			hadOriginal = true
			err = os.Rename(path, backup)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			err = statErr
		}
		if err == nil {
			err = os.Rename(temporary, path)
		}
		if err != nil && hadOriginal {
			_ = os.Rename(backup, path)
		}
		if err == nil && hadOriginal {
			_ = os.Remove(backup)
		}
	}
	if err != nil {
		_ = os.Remove(temporary)
	}
	return err
}

func verifyRegularFile(path string, expectedSize int64, expectedSHA string) (int64, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != expectedSize {
		return 0, fmt.Errorf("invalid file")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil || hex.EncodeToString(hash.Sum(nil)) != expectedSHA {
		return 0, fmt.Errorf("invalid hash")
	}
	return info.Size(), nil
}

func validateDownloadURL(u *url.URL) error {
	if u == nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return modelError("redirect_rejected", "The model source redirected to an unsafe URL.", false)
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && !strings.HasSuffix(host, ".githubusercontent.com") {
		return modelError("redirect_rejected", "The model source redirected outside the official host family.", false)
	}
	return nil
}

type modelLock struct {
	directory string
	token     string
}

type lockOwner struct {
	Token     string `json:"token"`
	PID       int    `json:"pid"`
	CreatedAt string `json:"created_at"`
}

func acquireModelLock(paths Paths, modelID string) (*modelLock, error) {
	directory := filepath.Join(paths.Locks, modelID+".lock")
	token := randomToken()
	for attempt := 0; attempt < 2; attempt++ {
		if err := os.Mkdir(directory, 0700); err == nil {
			owner := lockOwner{Token: token, PID: os.Getpid(), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
			if err := writeJSONAtomically(filepath.Join(directory, "owner.json"), owner, 0600); err != nil {
				_ = os.RemoveAll(directory)
				return nil, modelError("storage_unavailable", "Model operation lock could not be saved.", true)
			}
			return &modelLock{directory: directory, token: token}, nil
		} else if !errors.Is(err, os.ErrExist) {
			return nil, modelError("storage_unavailable", "Model operation lock is unavailable.", true)
		}
		data, readErr := os.ReadFile(filepath.Join(directory, "owner.json"))
		var owner lockOwner
		if readErr == nil && json.Unmarshal(data, &owner) == nil && owner.PID > 0 && processAlive(owner.PID) {
			return nil, modelError("model_busy", "Another model operation is already running.", true)
		}
		info, statErr := os.Stat(directory)
		if statErr != nil || time.Since(info.ModTime()) < 30*time.Second {
			return nil, modelError("model_busy", "Another model operation is already running.", true)
		}
		quarantine := directory + ".stale." + token
		if os.Rename(directory, quarantine) != nil {
			return nil, modelError("model_busy", "Another model operation is already running.", true)
		}
		movedData, movedErr := os.ReadFile(filepath.Join(quarantine, "owner.json"))
		var moved lockOwner
		if movedErr != nil || json.Unmarshal(movedData, &moved) != nil || moved != owner {
			_ = os.Rename(quarantine, directory)
			return nil, modelError("model_busy", "Another model operation is already running.", true)
		}
		_ = os.RemoveAll(quarantine)
	}
	return nil, modelError("model_busy", "Another model operation is already running.", true)
}

func (lock *modelLock) release() {
	data, err := os.ReadFile(filepath.Join(lock.directory, "owner.json"))
	var owner lockOwner
	if err == nil && json.Unmarshal(data, &owner) == nil && owner.Token == lock.token {
		quarantine := lock.directory + ".release." + lock.token
		if os.Rename(lock.directory, quarantine) != nil {
			return
		}
		movedData, movedErr := os.ReadFile(filepath.Join(quarantine, "owner.json"))
		var moved lockOwner
		if movedErr == nil && json.Unmarshal(movedData, &moved) == nil && moved.Token == lock.token {
			_ = os.RemoveAll(quarantine)
			return
		}
		_ = os.Rename(quarantine, lock.directory)
	}
}

func randomToken() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
