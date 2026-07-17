package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	customTemplateSchemaVersion = "1.0"
	maxTemplateNameLength       = 120
	maxTemplateDescription      = 1000
	maxTemplateInstructions     = 20000
	templateLockStaleAfter      = 5 * time.Minute
	templateLockOwnerFilename   = "owner.json"
)

type templateStoreLockOwner struct {
	Token     string `json:"token"`
	PID       int    `json:"pid"`
	CreatedAt int64  `json:"created_at_unix_nano"`
}

type templateStoreLock struct {
	path  string
	owner templateStoreLockOwner
}

type CustomTemplateRevision struct {
	Revision     int    `json:"revision"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	CreatedAt    string `json:"created_at"`
}

type CustomTemplate struct {
	TemplateID      string                   `json:"template_id"`
	Status          string                   `json:"status"`
	CurrentRevision int                      `json:"current_revision"`
	CreatedAt       string                   `json:"created_at"`
	UpdatedAt       string                   `json:"updated_at"`
	Revisions       []CustomTemplateRevision `json:"revisions"`
}

type CustomTemplateStore struct {
	SchemaVersion string           `json:"schema_version"`
	Templates     []CustomTemplate `json:"templates"`
}

type TemplateCatalog struct {
	Templates []PromptTemplate `json:"templates"`
	Issues    []TemplateIssue  `json:"issues"`
}

type TemplateIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CustomTemplateInput struct {
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

type UpdateCustomTemplateInput struct {
	TemplateID              string `json:"template_id"`
	ExpectedCurrentRevision int    `json:"expected_current_revision"`
	CustomTemplateInput
}

type DeleteCustomTemplateInput struct {
	TemplateID              string `json:"template_id"`
	ExpectedCurrentRevision int    `json:"expected_current_revision"`
}

func validateTemplateInput(input CustomTemplateInput) (CustomTemplateInput, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Description = strings.TrimSpace(input.Description)
	input.Instructions = strings.TrimSpace(input.Instructions)
	if input.DisplayName == "" || len([]rune(input.DisplayName)) > maxTemplateNameLength {
		return input, fmt.Errorf("invalid_template_input: display name is required and must be at most %d characters", maxTemplateNameLength)
	}
	if len([]rune(input.Description)) > maxTemplateDescription {
		return input, fmt.Errorf("invalid_template_input: description must be at most %d characters", maxTemplateDescription)
	}
	if input.Instructions == "" || len([]rune(input.Instructions)) > maxTemplateInstructions {
		return input, fmt.Errorf("invalid_template_input: instructions are required and must be at most %d characters", maxTemplateInstructions)
	}
	return input, nil
}

func emptyCustomTemplateStore() CustomTemplateStore {
	return CustomTemplateStore{SchemaVersion: customTemplateSchemaVersion, Templates: make([]CustomTemplate, 0)}
}

func loadCustomTemplateStore(path string) (CustomTemplateStore, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return CustomTemplateStore{}, fmt.Errorf("template_storage_unavailable: template storage path is invalid")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyCustomTemplateStore(), nil
	}
	if err != nil {
		return CustomTemplateStore{}, fmt.Errorf("template_storage_unavailable: custom template storage could not be read")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return CustomTemplateStore{}, fmt.Errorf("template_storage_unavailable: custom template storage is unsafe")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CustomTemplateStore{}, fmt.Errorf("template_storage_unavailable: custom template storage could not be read")
	}
	var store CustomTemplateStore
	if err := json.Unmarshal(data, &store); err != nil {
		return CustomTemplateStore{}, fmt.Errorf("corrupted_template_storage: custom template storage is invalid")
	}
	if store.SchemaVersion != customTemplateSchemaVersion {
		return CustomTemplateStore{}, fmt.Errorf("unsupported_template_schema: custom template storage schema is unsupported")
	}
	if err := validateCustomTemplateStore(store); err != nil {
		return CustomTemplateStore{}, err
	}
	return store, nil
}

func validateCustomTemplateStore(store CustomTemplateStore) error {
	seen := make(map[string]bool)
	for _, template := range store.Templates {
		if validateCustomTemplateID(template.TemplateID) != nil || seen[template.TemplateID] || template.Status != "active" && template.Status != "deleted" || template.CurrentRevision < 1 {
			return fmt.Errorf("corrupted_template_storage: custom template storage is invalid")
		}
		seen[template.TemplateID] = true
		if len(template.Revisions) != template.CurrentRevision {
			return fmt.Errorf("corrupted_template_storage: custom template revision history is invalid")
		}
		for index, revision := range template.Revisions {
			if revision.Revision != index+1 {
				return fmt.Errorf("corrupted_template_storage: custom template revision history is invalid")
			}
			if _, err := validateTemplateInput(CustomTemplateInput{DisplayName: revision.DisplayName, Description: revision.Description, Instructions: revision.Instructions}); err != nil {
				return fmt.Errorf("corrupted_template_storage: custom template revision is invalid")
			}
		}
	}
	return nil
}

func validateCustomTemplateID(id string) error {
	const prefix = "custom-"
	if len(id) != len(prefix)+32 || !strings.HasPrefix(id, prefix) {
		return fmt.Errorf("invalid_template_id: custom template ID is invalid")
	}
	for _, char := range id[len(prefix):] {
		if char < '0' || char > '9' {
			if char < 'a' || char > 'f' {
				return fmt.Errorf("invalid_template_id: custom template ID is invalid")
			}
		}
	}
	return nil
}

func saveCustomTemplateStore(path string, store CustomTemplateStore) error {
	if err := validateCustomTemplateStore(store); err != nil {
		return err
	}
	sort.Slice(store.Templates, func(i, j int) bool { return store.Templates[i].TemplateID < store.Templates[j].TemplateID })
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("template_storage_write_failed: custom template storage could not be serialized")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("template_storage_write_failed: custom template directory could not be created")
	}
	if err := WriteFileAtomically(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("template_storage_write_failed: custom template storage could not be committed")
	}
	return nil
}

func newTemplateStoreLockOwner() (templateStoreLockOwner, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return templateStoreLockOwner{}, fmt.Errorf("template_storage_write_failed: lock owner could not be created")
	}
	return templateStoreLockOwner{Token: hex.EncodeToString(raw[:]), PID: os.Getpid(), CreatedAt: time.Now().UnixNano()}, nil
}

func writeTemplateStoreLockOwner(lockPath string, owner templateStoreLockOwner) error {
	data, err := json.Marshal(owner)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(lockPath, templateLockOwnerFilename), data, 0600)
}

func readTemplateStoreLockOwner(lockPath string) (templateStoreLockOwner, error) {
	data, err := os.ReadFile(filepath.Join(lockPath, templateLockOwnerFilename))
	if err != nil {
		return templateStoreLockOwner{}, err
	}
	var owner templateStoreLockOwner
	if err := json.Unmarshal(data, &owner); err != nil || owner.Token == "" || owner.PID <= 0 || owner.CreatedAt <= 0 {
		return templateStoreLockOwner{}, fmt.Errorf("invalid lock owner")
	}
	return owner, nil
}

func recoverStaleTemplateStoreLock(lockPath string, staleAfter time.Duration) (bool, error) {
	info, err := os.Stat(lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	owner, ownerErr := readTemplateStoreLockOwner(lockPath)
	createdAt := info.ModTime()
	if ownerErr == nil {
		createdAt = time.Unix(0, owner.CreatedAt)
		if time.Since(createdAt) < staleAfter || processAlive(owner.PID) {
			return false, nil
		}
	} else if time.Since(createdAt) < staleAfter {
		return false, nil
	}
	quarantine := fmt.Sprintf("%s.stale-%d-%s", lockPath, os.Getpid(), mustRandomToken())
	if err := os.Rename(lockPath, quarantine); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	movedOwner, movedOwnerErr := readTemplateStoreLockOwner(quarantine)
	ownershipChanged := ownerErr == nil && (movedOwnerErr != nil || movedOwner.Token != owner.Token)
	if ownerErr != nil && movedOwnerErr == nil {
		ownershipChanged = true
	}
	if ownershipChanged {
		if _, statErr := os.Stat(lockPath); errors.Is(statErr, os.ErrNotExist) {
			_ = os.Rename(quarantine, lockPath)
		}
		return false, nil
	}
	if err := os.RemoveAll(quarantine); err != nil {
		return false, err
	}
	return true, nil
}

func mustRandomToken() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func acquireTemplateStoreLock(path string, staleAfter time.Duration) (*templateStoreLock, error) {
	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("template_storage_write_failed: custom template directory could not be created")
	}
	owner, err := newTemplateStoreLockOwner()
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 200; attempt++ {
		err := os.Mkdir(lockPath, 0700)
		if err == nil {
			if err := writeTemplateStoreLockOwner(lockPath, owner); err != nil {
				_ = os.RemoveAll(lockPath)
				return nil, fmt.Errorf("template_storage_write_failed: custom template lock owner could not be written")
			}
			return &templateStoreLock{path: lockPath, owner: owner}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("template_storage_write_failed: custom template storage could not be locked")
		}
		if _, err := recoverStaleTemplateStoreLock(lockPath, staleAfter); err != nil {
			return nil, fmt.Errorf("template_storage_write_failed: stale custom template lock could not be recovered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, fmt.Errorf("revision_conflict: custom template storage is busy")
}

func (lock *templateStoreLock) Release() error {
	owner, err := readTemplateStoreLockOwner(lock.path)
	if err != nil || owner.Token != lock.owner.Token {
		return nil
	}
	quarantine := fmt.Sprintf("%s.release-%d-%s", lock.path, os.Getpid(), lock.owner.Token)
	if err := os.Rename(lock.path, quarantine); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	movedOwner, err := readTemplateStoreLockOwner(quarantine)
	if err != nil || movedOwner.Token != lock.owner.Token {
		if _, statErr := os.Stat(lock.path); errors.Is(statErr, os.ErrNotExist) {
			_ = os.Rename(quarantine, lock.path)
		}
		return nil
	}
	return os.RemoveAll(quarantine)
}

func withTemplateStoreLock(path string, fn func() error) error {
	lock, err := acquireTemplateStoreLock(path, templateLockStaleAfter)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}

func newCustomTemplateID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("id_generation_failed: custom template ID could not be generated")
	}
	return "custom-" + hex.EncodeToString(raw[:]), nil
}

func templateFromCustom(template CustomTemplate) PromptTemplate {
	revision := template.Revisions[template.CurrentRevision-1]
	return PromptTemplate{ID: template.TemplateID, Revision: revision.Revision, DisplayName: map[string]string{"en": revision.DisplayName, "zh": revision.DisplayName}, Description: map[string]string{"en": revision.Description, "zh": revision.Description}, Instructions: revision.Instructions, ContentType: "text/markdown", Extension: ".md", BuiltIn: false, Ownership: "custom", Status: template.Status, CanEdit: template.Status == "active", CanDelete: template.Status == "active", CreatedAt: template.CreatedAt, UpdatedAt: template.UpdatedAt}
}

func ListPromptTemplateCatalog(path string) TemplateCatalog {
	catalog := TemplateCatalog{Templates: BuiltInTemplates(), Issues: make([]TemplateIssue, 0)}
	if strings.TrimSpace(path) == "" {
		return catalog
	}
	store, err := loadCustomTemplateStore(path)
	if err != nil {
		catalog.Issues = append(catalog.Issues, TemplateIssue{Code: "custom_storage_unavailable", Message: "Custom templates could not be loaded. Built-in templates remain available."})
		return catalog
	}
	custom := make([]PromptTemplate, 0)
	for _, template := range store.Templates {
		if template.Status == "active" {
			custom = append(custom, templateFromCustom(template))
		}
	}
	sort.Slice(custom, func(i, j int) bool {
		left, right := strings.ToLower(custom[i].DisplayName["en"]), strings.ToLower(custom[j].DisplayName["en"])
		if left != right {
			return left < right
		}
		return custom[i].ID < custom[j].ID
	})
	catalog.Templates = append(catalog.Templates, custom...)
	return catalog
}

func ResolveCustomPromptTemplate(path, id string) (PromptTemplate, error) {
	if err := validateCustomTemplateID(id); err != nil {
		return PromptTemplate{}, err
	}
	store, err := loadCustomTemplateStore(path)
	if err != nil {
		return PromptTemplate{}, err
	}
	for _, template := range store.Templates {
		if template.TemplateID == id {
			if template.Status != "active" {
				return PromptTemplate{}, fmt.Errorf("deleted_template: custom template is deleted")
			}
			return templateFromCustom(template), nil
		}
	}
	return PromptTemplate{}, fmt.Errorf("unknown_template: custom template does not exist")
}

func CreateCustomPromptTemplate(path string, input CustomTemplateInput) (PromptTemplate, error) {
	input, err := validateTemplateInput(input)
	if err != nil {
		return PromptTemplate{}, err
	}
	var created PromptTemplate
	err = withTemplateStoreLock(path, func() error {
		store, err := loadCustomTemplateStore(path)
		if err != nil {
			return err
		}
		id, err := newCustomTemplateID()
		if err != nil {
			return err
		}
		for _, item := range store.Templates {
			if item.TemplateID == id {
				return fmt.Errorf("id_collision: generated template ID already exists")
			}
		}
		now := nowFunc().UTC().Format(time.RFC3339)
		template := CustomTemplate{TemplateID: id, Status: "active", CurrentRevision: 1, CreatedAt: now, UpdatedAt: now, Revisions: []CustomTemplateRevision{{Revision: 1, DisplayName: input.DisplayName, Description: input.Description, Instructions: input.Instructions, CreatedAt: now}}}
		store.Templates = append(store.Templates, template)
		if err := saveCustomTemplateStore(path, store); err != nil {
			return err
		}
		created = templateFromCustom(template)
		return nil
	})
	return created, err
}

func UpdateCustomPromptTemplate(path string, input UpdateCustomTemplateInput) (PromptTemplate, error) {
	validated, err := validateTemplateInput(input.CustomTemplateInput)
	if err != nil {
		return PromptTemplate{}, err
	}
	if err := validateCustomTemplateID(input.TemplateID); err != nil {
		if _, builtIn := builtInTemplates[input.TemplateID]; builtIn {
			return PromptTemplate{}, fmt.Errorf("builtin_mutation_forbidden: built-in templates are immutable")
		}
		return PromptTemplate{}, err
	}
	var updated PromptTemplate
	err = withTemplateStoreLock(path, func() error {
		store, err := loadCustomTemplateStore(path)
		if err != nil {
			return err
		}
		for index := range store.Templates {
			template := &store.Templates[index]
			if template.TemplateID != input.TemplateID {
				continue
			}
			if template.Status != "active" {
				return fmt.Errorf("deleted_template: custom template is deleted")
			}
			if input.ExpectedCurrentRevision != template.CurrentRevision {
				return fmt.Errorf("revision_conflict: custom template changed; refresh and try again")
			}
			now := nowFunc().UTC().Format(time.RFC3339)
			next := template.CurrentRevision + 1
			template.Revisions = append(template.Revisions, CustomTemplateRevision{Revision: next, DisplayName: validated.DisplayName, Description: validated.Description, Instructions: validated.Instructions, CreatedAt: now})
			template.CurrentRevision, template.UpdatedAt = next, now
			if err := saveCustomTemplateStore(path, store); err != nil {
				return err
			}
			updated = templateFromCustom(*template)
			return nil
		}
		return fmt.Errorf("unknown_template: custom template does not exist")
	})
	return updated, err
}

func DeleteCustomPromptTemplate(path string, input DeleteCustomTemplateInput) error {
	if err := validateCustomTemplateID(input.TemplateID); err != nil {
		if _, builtIn := builtInTemplates[input.TemplateID]; builtIn {
			return fmt.Errorf("builtin_mutation_forbidden: built-in templates are immutable")
		}
		return err
	}
	return withTemplateStoreLock(path, func() error {
		store, err := loadCustomTemplateStore(path)
		if err != nil {
			return err
		}
		for index := range store.Templates {
			template := &store.Templates[index]
			if template.TemplateID != input.TemplateID {
				continue
			}
			if template.Status != "active" {
				return fmt.Errorf("deleted_template: custom template is deleted")
			}
			if input.ExpectedCurrentRevision != template.CurrentRevision {
				return fmt.Errorf("revision_conflict: custom template changed; refresh and try again")
			}
			template.Status, template.UpdatedAt = "deleted", nowFunc().UTC().Format(time.RFC3339)
			return saveCustomTemplateStore(path, store)
		}
		return fmt.Errorf("unknown_template: custom template does not exist")
	})
}
