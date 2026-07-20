package modelcatalog

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"runtime"
	"sort"
	"strings"
)

//go:embed models.v1.json
var embeddedCatalog []byte

const DefaultHuggingFaceEndpoint = "https://huggingface.co"

type Catalog struct {
	SchemaVersion   string  `json:"schema_version"`
	CatalogRevision string  `json:"catalog_revision"`
	Models          []Model `json:"models"`
}

type Model struct {
	ModelID        string            `json:"model_id"`
	DisplayMode    string            `json:"display_mode"`
	DisplayName    map[string]string `json:"display_name"`
	Engine         string            `json:"engine"`
	EngineRuntime  string            `json:"engine_runtime"`
	RuntimeVersion *string           `json:"runtime_version"`
	ModelVersion   string            `json:"model_version"`
	Platforms      []string          `json:"platforms"`
	Source         Source            `json:"source"`
	Archive        Archive           `json:"archive"`
	InstalledFiles []FileIdentity    `json:"installed_files"`
	License        License           `json:"license"`
}

type Source struct {
	Kind       string `json:"kind"`
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	URL        string `json:"url"`
}

type Archive struct {
	Filename      string         `json:"filename"`
	Type          string         `json:"type"`
	SizeBytes     int64          `json:"size_bytes"`
	SHA256        string         `json:"sha256"`
	RootDirectory string         `json:"root_directory"`
	Entries       []ArchiveEntry `json:"entries"`
}

type ArchiveEntry struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type FileIdentity struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type License struct {
	NoticeID                     string `json:"notice_id"`
	RedistributionReviewRequired bool   `json:"redistribution_review_required"`
}

func Load() (Catalog, error) {
	var catalog Catalog
	if err := json.Unmarshal(embeddedCatalog, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("model_catalog_invalid: decode catalog")
	}
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func (catalog Catalog) Validate() error {
	if catalog.SchemaVersion != "1.0" || catalog.CatalogRevision == "" {
		return fmt.Errorf("model_catalog_invalid: unsupported catalog identity")
	}
	seen := map[string]bool{}
	for _, model := range catalog.Models {
		if !safePathSegment(model.ModelID) || seen[model.ModelID] {
			return fmt.Errorf("model_catalog_invalid: duplicate or empty model ID")
		}
		seen[model.ModelID] = true
		if !safePathSegment(model.ModelVersion) || model.Engine == "" || model.EngineRuntime == "" {
			return fmt.Errorf("model_catalog_invalid: incomplete model identity")
		}
		if len(model.Platforms) == 0 || model.Source.Repository == "" || model.Source.Revision == "" || strings.Contains(strings.ToLower(model.Source.URL), "latest") {
			return fmt.Errorf("model_catalog_invalid: unpinned source")
		}
		u, err := url.Parse(model.Source.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("model_catalog_invalid: unsafe source")
		}
		if model.Archive.Type != "tar.bz2" || model.Archive.SizeBytes <= 0 || !validSHA(model.Archive.SHA256) || !safePathSegment(model.Archive.RootDirectory) || !safePathSegment(model.Archive.Filename) {
			return fmt.Errorf("model_catalog_invalid: invalid archive identity")
		}
		entrySeen := map[string]bool{}
		for _, entry := range model.Archive.Entries {
			if !safeRelative(entry.Path) || entrySeen[strings.ToLower(entry.Path)] {
				return fmt.Errorf("model_catalog_invalid: unsafe or duplicate archive entry")
			}
			entrySeen[strings.ToLower(entry.Path)] = true
			if entry.Type != "file" && entry.Type != "directory" {
				return fmt.Errorf("model_catalog_invalid: invalid archive entry type")
			}
			if entry.Type == "file" && (entry.SizeBytes < 0 || !validSHA(entry.SHA256)) {
				return fmt.Errorf("model_catalog_invalid: invalid archive file identity")
			}
		}
		installedSeen := map[string]bool{}
		for _, file := range model.InstalledFiles {
			if !safeRelative(file.Path) || installedSeen[strings.ToLower(file.Path)] || file.SizeBytes <= 0 || !validSHA(file.SHA256) {
				return fmt.Errorf("model_catalog_invalid: invalid installed file identity")
			}
			installedSeen[strings.ToLower(file.Path)] = true
			if !entrySeen[strings.ToLower(file.Path)] {
				return fmt.Errorf("model_catalog_invalid: installed file missing from archive")
			}
		}
	}
	return nil
}

func safePathSegment(value string) bool {
	return safeRelative(value) && !strings.Contains(value, "/")
}

func (catalog Catalog) Model(modelID string) (Model, error) {
	for _, model := range catalog.Models {
		if model.ModelID == modelID {
			return model, nil
		}
	}
	return Model{}, fmt.Errorf("unsupported_model: model is not managed by Voxly")
}

func CurrentPlatform() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	}
	return runtime.GOOS + "-" + arch
}

func SupportsCurrentPlatform(model Model) bool {
	want := CurrentPlatform()
	for _, platform := range model.Platforms {
		if platform == want {
			return true
		}
	}
	return false
}

func NormalizeHuggingFaceEndpoint(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = DefaultHuggingFaceEndpoint
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("source_rejected: Hugging Face endpoint must be an HTTPS origin")
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("source_rejected: Hugging Face endpoint cannot include a path")
	}
	return "https://" + strings.ToLower(u.Host), nil
}

func RewriteHuggingFaceOrigin(sourceURL, endpoint string) (string, error) {
	source, err := url.Parse(sourceURL)
	if err != nil || source.Hostname() != "huggingface.co" || source.Scheme != "https" || source.User != nil {
		return sourceURL, nil
	}
	origin, err := NormalizeHuggingFaceEndpoint(endpoint)
	if err != nil {
		return "", err
	}
	replacement, _ := url.Parse(origin)
	source.Scheme, source.Host = replacement.Scheme, replacement.Host
	return source.String(), nil
}

func safeRelative(value string) bool {
	return value != "" && !strings.Contains(value, "\\") && path.Clean(value) == value && value != "." && !strings.HasPrefix(value, "../") && !strings.HasPrefix(value, "/")
}

func validSHA(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func SortedModelIDs(catalog Catalog) []string {
	ids := make([]string, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		ids = append(ids, model.ModelID)
	}
	sort.Strings(ids)
	return ids
}
