# Voxly branding and storage namespace

Voxly / 声笺 uses the release identity `com.skybridgem.voxly` and version
`0.1.0-alpha`. The repository, Go sidecar, Rust crate, CLI command, Task
manifest names, and Task folder formats intentionally retain their existing
internal names.

## Branding audit

Must change:

- Tauri product name, window title, bundle identifier, version and icons;
- desktop header, subtitle, About information and localized product copy;
- README title and current product description;
- secure credential service for newly saved credentials.

May remain internal:

- repository name `meet-asr`;
- Go binary `meeting-asr-go` and existing CLI commands;
- Rust crate/package names;
- development test fixture names and historical documentation;
- the existing default output folder name, to avoid silently changing where
  established users find Tasks.

Must not change:

- TaskManifest schema or Task folder identity;
- user-selected output roots, Tasks, transcripts and generated artifacts;
- custom-template schema;
- provider semantics and runtime behavior.

## Namespace paths

Tauri derives the current config and data roots from
`com.skybridgem.voxly`. Legacy roots are the sibling locations derived from
`com.meeting-asr.desktop-experiment`. Exact platform prefixes remain owned by
Tauri:

```text
<app_config_dir>/com.meeting-asr.desktop-experiment
  -> <app_config_dir>/com.skybridgem.voxly

<app_data_dir>/com.meeting-asr.desktop-experiment
  -> <app_data_dir>/com.skybridgem.voxly
```

The current migration allowlist is deliberately limited to:

```text
desktop-settings.json
templates/custom_templates.json
```

No model, cache, log, Task, output-root or arbitrary file directory is
allowlisted.

## Startup migration

Migration runs in Tauri setup before commands can load settings, templates or
credential readiness. It runs only when both resolved current roots end in the
exact release identifier; isolated smoke identifiers do not import real legacy
data.

The migration:

1. reads a versioned marker when present;
2. serializes execution with an app-owned migration lock;
3. inspects only allowlisted regular files and rejects symbolic links;
4. copies through a same-directory temporary file, syncs it, preserves source
   permissions and atomically renames it;
5. treats an existing new-namespace file as authoritative and preserves the
   legacy file;
6. migrates a missing allowlisted file even when another target already
   exists, allowing partial prior migrations to recover;
7. writes `.voxly-namespace-migration-v1.json` atomically;
8. never deletes the old namespace.

Unknown files are ignored. Migration issues contain only allowlist labels and
never file contents. Failure remains recoverable and does not prevent launch.

## Credentials

New credentials use the secure-store service `com.skybridgem.voxly`. Reads
check that service first and then the legacy
`com.meeting-asr.desktop-experiment` service. A legacy value is copied through
the operating system secure-store API when possible; it is never written to a
plain-text migration file, returned to the frontend or logged. The legacy
entry is preserved. If the secure store cannot migrate it, compatibility reads
continue to work where the platform permits; otherwise the user may need to
re-enter the credential.

## Icon source

`voxly-app-icon-concept.png` is a visual reference sheet only.
`voxly-app-icon-master.png` is a separately generated 1024×1024 production
master. Tauri platform PNG, ICNS and ICO assets are generated from that master,
not cropped from the reference sheet.

## Future model storage

Future local model management must use the new Tauri app-data namespace. This
PR intentionally creates no model directories and migrates no speculative
model data.

## Isolated smoke

Build and launch the branded UI without touching release-namespace data:

```bash
cd /Users/yudi/meet-asr
cd core-go && go build -o /tmp/voxly-pr47-smoke-go . && cd ..
printf '%s\n' '{"productName":"Voxly PR47 Smoke","identifier":"com.skybridgem.voxly.pr47.smoke"}' > /tmp/voxly-pr47-smoke-config.json
MEETING_ASR_GO_BINARY=/tmp/voxly-pr47-smoke-go npm --prefix desktop run tauri -- dev --config /tmp/voxly-pr47-smoke-config.json
```

The exact smoke identifier is intentionally excluded from legacy migration.
Exercise migration against synthetic temporary paths with:

```bash
cargo test --manifest-path desktop/src-tauri/Cargo.toml app_namespace::tests:: -- --nocapture
```

This covers empty, settings-only/templates, combined, conflict, partial,
repeated, corrupt-marker, permissions, symlink, lock ownership and smoke
isolation behavior without reading real application directories or credentials.
