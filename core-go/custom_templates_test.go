package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func customStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "templates", "custom_templates.json")
}

func TestCustomTemplateIDGrammarAndArtifactPathSafety(t *testing.T) {
	valid := "custom-0123456789abcdef0123456789abcdef"
	if err := validateCustomTemplateID(valid); err != nil {
		t.Fatalf("valid ID rejected: %v", err)
	}
	invalid := []string{
		"custom-../../escape",
		"custom-/absolute",
		"custom-abc",
		"custom-0123456789abcdef0123456789abcde",
		"custom-0123456789abcdef0123456789abc",
		"custom-0123456789ABCDEF0123456789ABCDEF",
		"custom-0123456789abcdef0123456789abcdeg",
	}
	root := t.TempDir()
	taskDir := filepath.Join(root, "task")
	if err := os.Mkdir(taskDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, id := range invalid {
		t.Run(strings.ReplaceAll(id, "/", "_"), func(t *testing.T) {
			if err := validateCustomTemplateID(id); err == nil {
				t.Fatalf("invalid ID accepted: %q", id)
			}
			storePath := filepath.Join(root, strings.ReplaceAll(strings.ReplaceAll(id, "/", "_"), ".", "dot")+".json")
			store := CustomTemplateStore{SchemaVersion: customTemplateSchemaVersion, Templates: []CustomTemplate{{TemplateID: id, Status: "active", CurrentRevision: 1, Revisions: []CustomTemplateRevision{{Revision: 1, DisplayName: "Unsafe", Instructions: "Unsafe", CreatedAt: time.Now().UTC().Format(time.RFC3339)}}}}}
			data, err := json.Marshal(store)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(storePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadCustomTemplateStore(storePath); err == nil {
				t.Fatalf("corrupted storage accepted ID %q", id)
			}
			if _, _, err := validatedGeneratedArtifactPath(taskDir, id, "gen_0123456789abcdef0123456789abcdef", ".md"); err == nil {
				t.Fatalf("artifact path accepted ID %q", id)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "escape")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escape path was created: %v", err)
	}
}

func TestGeneratedArtifactPathRejectsExistingSymlinkEscapeBeforeWrite(t *testing.T) {
	root := t.TempDir()
	taskDir, outside := filepath.Join(root, "task"), filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(taskDir, "generated"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	id := "custom-0123456789abcdef0123456789abcdef"
	if err := os.Symlink(outside, filepath.Join(taskDir, "generated", id)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := validatedGeneratedArtifactPath(taskDir, id, "gen_0123456789abcdef0123456789abcdef", ".md"); err == nil {
		t.Fatal("symlink escape was accepted")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside directory changed: entries=%v err=%v", entries, err)
	}
}

func TestTemplateStoreLockNormalAcquireReleaseAndActiveProtection(t *testing.T) {
	path := customStorePath(t)
	lock, err := acquireTemplateStoreLock(path, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireTemplateStoreLock(path, time.Hour); err == nil || !strings.Contains(err.Error(), "revision_conflict") {
		t.Fatalf("active lock was stolen: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock remains after release: %v", err)
	}
}

func TestTemplateStoreLockDoesNotStealOldLiveOwner(t *testing.T) {
	path := customStorePath(t)
	lock, err := acquireTemplateStoreLock(path, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	lock.owner.CreatedAt = time.Now().Add(-time.Hour).UnixNano()
	if err := writeTemplateStoreLockOwner(lock.path, lock.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireTemplateStoreLock(path, time.Millisecond); err == nil || !strings.Contains(err.Error(), "revision_conflict") {
		t.Fatalf("live lock was stolen: %v", err)
	}
	_ = lock.Release()
}

func TestTemplateStoreLockRecoversCrashOrphan(t *testing.T) {
	path := customStorePath(t)
	lockPath := path + ".lock"
	if err := os.MkdirAll(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	orphan := templateStoreLockOwner{Token: "orphan", PID: 1 << 30, CreatedAt: time.Now().Add(-time.Hour).UnixNano()}
	if err := writeTemplateStoreLockOwner(lockPath, orphan); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireTemplateStoreLock(path, time.Millisecond)
	if err != nil {
		t.Fatalf("orphan lock was not recovered: %v", err)
	}
	defer lock.Release()
	if lock.owner.Token == orphan.Token {
		t.Fatal("orphan ownership was reused")
	}
}

func TestTemplateStoreLockRecoversOldCorruptMetadata(t *testing.T) {
	path := customStorePath(t)
	lockPath := path + ".lock"
	if err := os.MkdirAll(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireTemplateStoreLock(path, time.Millisecond)
	if err != nil {
		t.Fatalf("corrupt orphan lock was not recovered: %v", err)
	}
	_ = lock.Release()
}

func TestTemplateStoreUnlockDoesNotRemoveAnotherOwner(t *testing.T) {
	path := customStorePath(t)
	lock, err := acquireTemplateStoreLock(path, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := lock.path + ".old"
	if err := os.Rename(lock.path, oldPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lock.path, 0700); err != nil {
		t.Fatal(err)
	}
	replacement := templateStoreLockOwner{Token: "replacement", PID: os.Getpid(), CreatedAt: time.Now().UnixNano()}
	if err := writeTemplateStoreLockOwner(lock.path, replacement); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	owner, err := readTemplateStoreLockOwner(lock.path)
	if err != nil || owner.Token != replacement.Token {
		t.Fatalf("replacement lock was removed: owner=%#v err=%v", owner, err)
	}
}

func TestCustomTemplateCreateEditDeletePreservesRevisions(t *testing.T) {
	path := customStorePath(t)
	created, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: "Weekly report", Instructions: "Summarize weekly progress."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.ID, "custom-") || created.Revision != 1 || created.BuiltIn || !created.CanEdit || !created.CanDelete {
		t.Fatalf("created = %#v", created)
	}
	updated, err := UpdateCustomPromptTemplate(path, UpdateCustomTemplateInput{TemplateID: created.ID, ExpectedCurrentRevision: 1, CustomTemplateInput: CustomTemplateInput{DisplayName: "Weekly report v2", Description: "Team update", Instructions: "Summarize progress and risks."}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision = %d", updated.Revision)
	}
	store, err := loadCustomTemplateStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Templates[0].Revisions) != 2 || store.Templates[0].Revisions[0].DisplayName != "Weekly report" {
		t.Fatalf("store = %#v", store)
	}
	if err := DeleteCustomPromptTemplate(path, DeleteCustomTemplateInput{TemplateID: created.ID, ExpectedCurrentRevision: 2}); err != nil {
		t.Fatal(err)
	}
	catalog := ListPromptTemplateCatalog(path)
	if len(catalog.Templates) != 4 {
		t.Fatalf("catalog = %#v", catalog)
	}
	store, err = loadCustomTemplateStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Templates[0].Status != "deleted" || len(store.Templates[0].Revisions) != 2 {
		t.Fatalf("deleted store = %#v", store)
	}
	if _, err := ResolveCustomPromptTemplate(path, created.ID); err == nil || !strings.Contains(err.Error(), "deleted_template") {
		t.Fatalf("resolve deleted error = %v", err)
	}
}

func TestCustomTemplateCatalogBuiltinsFirstAndCustomDeterministic(t *testing.T) {
	path := customStorePath(t)
	for _, name := range []string{"Zulu", "Alpha"} {
		if _, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: name, Instructions: "Safe instructions"}); err != nil {
			t.Fatal(err)
		}
	}
	catalog := ListPromptTemplateCatalog(path)
	if len(catalog.Templates) != 6 {
		t.Fatalf("catalog = %#v", catalog)
	}
	for index := 0; index < 4; index++ {
		if !catalog.Templates[index].BuiltIn || catalog.Templates[index].CanEdit || catalog.Templates[index].CanDelete {
			t.Fatalf("builtin = %#v", catalog.Templates[index])
		}
	}
	if catalog.Templates[4].DisplayName["en"] != "Alpha" || catalog.Templates[5].DisplayName["en"] != "Zulu" {
		t.Fatalf("ordering = %#v", catalog.Templates)
	}
}

func TestCustomTemplateValidationAndCorruptionSafety(t *testing.T) {
	path := customStorePath(t)
	if _, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: " ", Instructions: "x"}); err == nil {
		t.Fatal("expected blank-name error")
	}
	if _, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: "Name", Instructions: " "}); err == nil {
		t.Fatal("expected blank-instructions error")
	}
	if _, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: strings.Repeat("x", maxTemplateNameLength+1), Instructions: "x"}); err == nil {
		t.Fatal("expected long-name error")
	}
	if _, err := UpdateCustomPromptTemplate(path, UpdateCustomTemplateInput{TemplateID: "meeting-minutes", ExpectedCurrentRevision: 1, CustomTemplateInput: CustomTemplateInput{DisplayName: "No", Instructions: "No"}}); err == nil || !strings.Contains(err.Error(), "builtin_mutation_forbidden") {
		t.Fatalf("builtin update error = %v", err)
	}
	if err := DeleteCustomPromptTemplate(path, DeleteCustomTemplateInput{TemplateID: "meeting-minutes", ExpectedCurrentRevision: 1}); err == nil || !strings.Contains(err.Error(), "builtin_mutation_forbidden") {
		t.Fatalf("builtin delete error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":"9.0","templates":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	catalog := ListPromptTemplateCatalog(path)
	if len(catalog.Templates) != 4 || len(catalog.Issues) != 1 {
		t.Fatalf("safe fallback = %#v", catalog)
	}
}

func TestCustomTemplateDuplicateNamesAllowedAndDeletedIDNotReused(t *testing.T) {
	path := customStorePath(t)
	first, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: "Duplicate", Instructions: "One"})
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteCustomPromptTemplate(path, DeleteCustomTemplateInput{TemplateID: first.ID, ExpectedCurrentRevision: 1}); err != nil {
		t.Fatal(err)
	}
	second, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: "Duplicate", Instructions: "Two"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("deleted custom template ID was reused")
	}
}

func TestCustomTemplateAtomicWriteFailureLeavesNoStore(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "custom_templates.json")
	if _, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: "Safe", Instructions: "Safe"}); err == nil {
		t.Fatal("expected storage write failure")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("partial store exists")
	}
}

func TestCustomTemplateConcurrentEditUsesRevisionConflict(t *testing.T) {
	path := customStorePath(t)
	created, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: "One", Instructions: "Initial"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errorsFound := make(chan error, 2)
	for _, name := range []string{"Two", "Three"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_, err := UpdateCustomPromptTemplate(path, UpdateCustomTemplateInput{TemplateID: created.ID, ExpectedCurrentRevision: 1, CustomTemplateInput: CustomTemplateInput{DisplayName: name, Instructions: "Updated"}})
			errorsFound <- err
		}(name)
	}
	wg.Wait()
	close(errorsFound)
	successes, conflicts := 0, 0
	for err := range errorsFound {
		if err == nil {
			successes++
		} else if strings.Contains(err.Error(), "revision_conflict") {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestCustomTemplateStorageDeterministicOwnerOnlyAndNoSecrets(t *testing.T) {
	path := customStorePath(t)
	if _, err := CreateCustomPromptTemplate(path, CustomTemplateInput{DisplayName: "Safe", Description: "Description", Instructions: "Instructions"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CustomTemplateStore
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != customTemplateSchemaVersion || strings.Contains(string(data), "API_KEY") {
		t.Fatalf("storage = %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
	}
}

func TestCustomTemplateCLIProducesMachineReadableCatalogAndCRUD(t *testing.T) {
	path := customStorePath(t)
	var created bytes.Buffer
	if err := runCreateCustomTemplate([]string{"--templates-file", path}, strings.NewReader(`{"display_name":"CLI template","description":"","instructions":"Do the work"}`), &created); err != nil {
		t.Fatal(err)
	}
	var template PromptTemplate
	if err := json.Unmarshal(created.Bytes(), &template); err != nil {
		t.Fatal(err)
	}
	var catalog bytes.Buffer
	if err := runListTemplates([]string{"--templates-file", path}, &catalog); err != nil {
		t.Fatal(err)
	}
	var decoded TemplateCatalog
	if err := json.Unmarshal(catalog.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Templates) != 5 || decoded.Templates[4].ID != template.ID {
		t.Fatalf("catalog = %#v", decoded)
	}
	if err := runDeleteCustomTemplate([]string{"--templates-file", path}, strings.NewReader(fmt.Sprintf(`{"template_id":%q,"expected_current_revision":1}`, template.ID)), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func TestCustomTemplateStorageIgnoresSameVersionUnknownFields(t *testing.T) {
	path := customStorePath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data := `{"schema_version":"1.0","future":"safe","templates":[]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := loadCustomTemplateStore(path)
	if err != nil || len(store.Templates) != 0 {
		t.Fatalf("store=%#v err=%v", store, err)
	}
}
