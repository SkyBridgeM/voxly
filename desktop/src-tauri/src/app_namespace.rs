use serde::{Deserialize, Serialize};
use std::fs::{self, OpenOptions};
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

pub(crate) const CURRENT_APP_IDENTIFIER: &str = "com.skybridgem.voxly";
pub(crate) const LEGACY_APP_IDENTIFIER: &str = "com.meeting-asr.desktop-experiment";
pub(crate) const MIGRATION_MARKER_FILE: &str = ".voxly-namespace-migration-v1.json";
const MIGRATION_LOCK_DIR: &str = ".voxly-namespace-migration-v1.lock";
const SETTINGS_FILE: &str = "desktop-settings.json";
const TEMPLATE_FILE: &str = "custom_templates.json";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct AppNamespacePaths {
    pub current_config: PathBuf,
    pub current_data: PathBuf,
    pub legacy_config: PathBuf,
    pub legacy_data: PathBuf,
    pub marker: PathBuf,
}

impl AppNamespacePaths {
    pub(crate) fn from_current(current_config: PathBuf, current_data: PathBuf) -> io::Result<Self> {
        let legacy_config = sibling_namespace(&current_config, LEGACY_APP_IDENTIFIER)?;
        let legacy_data = sibling_namespace(&current_data, LEGACY_APP_IDENTIFIER)?;
        let marker = current_config.join(MIGRATION_MARKER_FILE);
        Ok(Self {
            current_config,
            current_data,
            legacy_config,
            legacy_data,
            marker,
        })
    }

    pub(crate) fn is_release_namespace(&self) -> bool {
        self.current_config
            .file_name()
            .and_then(|value| value.to_str())
            == Some(CURRENT_APP_IDENTIFIER)
            && self
                .current_data
                .file_name()
                .and_then(|value| value.to_str())
                == Some(CURRENT_APP_IDENTIFIER)
    }
}

fn sibling_namespace(current: &Path, identifier: &str) -> io::Result<PathBuf> {
    current
        .parent()
        .map(|parent| parent.join(identifier))
        .ok_or_else(|| io::Error::new(io::ErrorKind::InvalidInput, "App namespace has no parent"))
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub(crate) struct MigrationReport {
    schema_version: String,
    source_identifier: String,
    target_identifier: String,
    pub(crate) completed: bool,
    copied: Vec<String>,
    conflicts: Vec<String>,
    pub(crate) issues: Vec<String>,
}

impl MigrationReport {
    fn new() -> Self {
        Self {
            schema_version: "1.0".to_string(),
            source_identifier: LEGACY_APP_IDENTIFIER.to_string(),
            target_identifier: CURRENT_APP_IDENTIFIER.to_string(),
            completed: false,
            copied: Vec::new(),
            conflicts: Vec::new(),
            issues: Vec::new(),
        }
    }
}

#[derive(Debug)]
struct MigrationLock {
    path: PathBuf,
    owner: String,
}

impl Drop for MigrationLock {
    fn drop(&mut self) {
        if fs::read_to_string(self.path.join("owner")).ok().as_deref() == Some(&self.owner) {
            let _ = fs::remove_dir_all(&self.path);
        }
    }
}

fn acquire_lock(config_dir: &Path) -> io::Result<MigrationLock> {
    fs::create_dir_all(config_dir)?;
    let path = config_dir.join(MIGRATION_LOCK_DIR);
    match fs::create_dir(&path) {
        Ok(()) => {}
        Err(error) if error.kind() == io::ErrorKind::AlreadyExists => {
            let stale = fs::symlink_metadata(&path).ok().is_some_and(|metadata| {
                metadata.is_dir()
                    && !metadata.file_type().is_symlink()
                    && metadata
                        .modified()
                        .ok()
                        .and_then(|modified| SystemTime::now().duration_since(modified).ok())
                        .is_some_and(|age| age.as_secs() > 300)
            });
            if !stale {
                return Err(io::Error::new(
                    io::ErrorKind::WouldBlock,
                    "Namespace migration is already running",
                ));
            }
            fs::remove_dir_all(&path)?;
            fs::create_dir(&path)?;
        }
        Err(error) => return Err(error),
    }
    let owner = format!("{}-{}", std::process::id(), unique_suffix());
    fs::write(path.join("owner"), &owner)?;
    Ok(MigrationLock { path, owner })
}

fn unique_suffix() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_nanos())
        .unwrap_or_default()
}

pub(crate) fn migrate_app_namespace(paths: &AppNamespacePaths) -> MigrationReport {
    let mut report = MigrationReport::new();
    if !paths.is_release_namespace() {
        report.completed = true;
        return report;
    }
    if [&paths.current_config, &paths.legacy_config]
        .into_iter()
        .filter_map(|path| fs::symlink_metadata(path).ok())
        .any(|metadata| metadata.file_type().is_symlink())
    {
        report
            .issues
            .push("Namespace migration rejected an unsafe application directory.".to_string());
        return report;
    }
    if let Ok(marker) = fs::read_to_string(&paths.marker) {
        if let Ok(previous) = serde_json::from_str::<MigrationReport>(&marker) {
            if previous.completed
                && previous.source_identifier == LEGACY_APP_IDENTIFIER
                && previous.target_identifier == CURRENT_APP_IDENTIFIER
            {
                return previous;
            }
        }
    }

    let _lock = match acquire_lock(&paths.current_config) {
        Ok(lock) => lock,
        Err(_) => {
            report
                .issues
                .push("Namespace migration could not acquire its application lock.".to_string());
            return report;
        }
    };

    migrate_known_file(
        &paths.legacy_config.join(SETTINGS_FILE),
        &paths.current_config.join(SETTINGS_FILE),
        SETTINGS_FILE,
        &mut report,
    );
    migrate_known_file(
        &paths.legacy_config.join("templates").join(TEMPLATE_FILE),
        &paths.current_config.join("templates").join(TEMPLATE_FILE),
        "templates/custom_templates.json",
        &mut report,
    );

    // No current app-data files are allowlisted. Keeping both roots in this
    // abstraction freezes the future model-storage namespace without copying
    // unknown legacy data.
    report.completed = true;
    if write_marker_atomically(&paths.marker, &report).is_err() {
        report.completed = false;
        report
            .issues
            .push("Namespace migration completed but its marker could not be saved.".to_string());
    }
    report
}

fn migrate_known_file(source: &Path, target: &Path, label: &str, report: &mut MigrationReport) {
    if source
        .parent()
        .and_then(|parent| fs::symlink_metadata(parent).ok())
        .is_some_and(|metadata| metadata.file_type().is_symlink())
    {
        report
            .issues
            .push(format!("Legacy {label} has an unsafe parent directory."));
        return;
    }
    let source_metadata = match fs::symlink_metadata(source) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == io::ErrorKind::NotFound => return,
        Err(_) => {
            report
                .issues
                .push(format!("Could not inspect legacy {label}."));
            return;
        }
    };
    if source_metadata.file_type().is_symlink() || !source_metadata.is_file() {
        report
            .issues
            .push(format!("Legacy {label} is not a safe regular file."));
        return;
    }
    if fs::symlink_metadata(target).is_ok() {
        report.conflicts.push(label.to_string());
        return;
    }
    if copy_file_atomically(source, target, &source_metadata.permissions()).is_err() {
        report
            .issues
            .push(format!("Could not migrate legacy {label}."));
        return;
    }
    report.copied.push(label.to_string());
}

fn copy_file_atomically(
    source: &Path,
    target: &Path,
    permissions: &fs::Permissions,
) -> io::Result<()> {
    let parent = target
        .parent()
        .ok_or_else(|| io::Error::new(io::ErrorKind::InvalidInput, "Target has no parent"))?;
    if let Ok(metadata) = fs::symlink_metadata(parent) {
        if metadata.file_type().is_symlink() || !metadata.is_dir() {
            return Err(io::Error::new(
                io::ErrorKind::InvalidInput,
                "Unsafe target parent",
            ));
        }
    } else {
        fs::create_dir_all(parent)?;
    }
    if fs::symlink_metadata(target).is_ok() {
        return Err(io::Error::new(
            io::ErrorKind::AlreadyExists,
            "Target exists",
        ));
    }

    let temporary = parent.join(format!(".namespace-copy-{}.tmp", unique_suffix()));
    let result = (|| {
        let mut input = OpenOptions::new().read(true).open(source)?;
        let mut output = OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&temporary)?;
        let mut buffer = [0_u8; 16 * 1024];
        loop {
            let count = input.read(&mut buffer)?;
            if count == 0 {
                break;
            }
            output.write_all(&buffer[..count])?;
        }
        output.sync_all()?;
        fs::set_permissions(&temporary, permissions.clone())?;
        if fs::symlink_metadata(target).is_ok() {
            return Err(io::Error::new(
                io::ErrorKind::AlreadyExists,
                "Target exists",
            ));
        }
        fs::rename(&temporary, target)
    })();
    if result.is_err() {
        let _ = fs::remove_file(&temporary);
    }
    result
}

fn write_marker_atomically(path: &Path, report: &MigrationReport) -> io::Result<()> {
    let parent = path
        .parent()
        .ok_or_else(|| io::Error::new(io::ErrorKind::InvalidInput, "Marker has no parent"))?;
    fs::create_dir_all(parent)?;
    let temporary = parent.join(format!(".namespace-marker-{}.tmp", unique_suffix()));
    let contents = serde_json::to_vec_pretty(report)?;
    let mut file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&temporary)?;
    file.write_all(&contents)?;
    file.write_all(b"\n")?;
    file.sync_all()?;
    if fs::symlink_metadata(path).is_err() {
        return fs::rename(&temporary, path);
    }

    // Windows cannot rename over an existing marker. Move the known marker
    // aside under the migration lock, commit the replacement, then remove the
    // backup. Restore it if the replacement cannot be committed.
    let backup = parent.join(format!(".namespace-marker-{}.previous", unique_suffix()));
    fs::rename(path, &backup)?;
    match fs::rename(&temporary, path) {
        Ok(()) => {
            let _ = fs::remove_file(backup);
            Ok(())
        }
        Err(error) => {
            let _ = fs::rename(backup, path);
            let _ = fs::remove_file(temporary);
            Err(error)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_paths(name: &str) -> AppNamespacePaths {
        let root = std::env::temp_dir().join(format!(
            "voxly-namespace-{name}-{}-{}",
            std::process::id(),
            unique_suffix()
        ));
        AppNamespacePaths::from_current(
            root.join("config").join(CURRENT_APP_IDENTIFIER),
            root.join("data").join(CURRENT_APP_IDENTIFIER),
        )
        .unwrap()
    }

    fn write(path: &Path, value: &str) {
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(path, value).unwrap();
    }

    #[test]
    fn migrates_only_allowlisted_files_and_is_idempotent() {
        let paths = temp_paths("allowlist");
        write(&paths.legacy_config.join(SETTINGS_FILE), "settings");
        write(
            &paths.legacy_config.join("templates").join(TEMPLATE_FILE),
            "templates",
        );
        write(&paths.legacy_config.join("unknown.txt"), "unknown");

        let first = migrate_app_namespace(&paths);
        assert!(first.completed);
        assert_eq!(first.copied.len(), 2);
        assert_eq!(
            fs::read_to_string(paths.current_config.join(SETTINGS_FILE)).unwrap(),
            "settings"
        );
        assert!(!paths.current_config.join("unknown.txt").exists());
        assert_eq!(migrate_app_namespace(&paths), first);
        assert!(paths.legacy_config.join(SETTINGS_FILE).exists());
    }

    #[test]
    fn migrates_settings_only() {
        let paths = temp_paths("settings-only");
        write(&paths.legacy_config.join(SETTINGS_FILE), "settings");
        let report = migrate_app_namespace(&paths);
        assert_eq!(report.copied, vec![SETTINGS_FILE]);
        assert!(!paths.current_config.join("templates").exists());
    }

    #[test]
    fn migrates_templates_only() {
        let paths = temp_paths("templates-only");
        write(
            &paths.legacy_config.join("templates").join(TEMPLATE_FILE),
            "templates",
        );
        let report = migrate_app_namespace(&paths);
        assert_eq!(report.copied, vec!["templates/custom_templates.json"]);
        assert!(!paths.current_config.join(SETTINGS_FILE).exists());
    }

    #[test]
    fn no_legacy_data_completes_without_creating_app_data_content() {
        let paths = temp_paths("empty");
        let report = migrate_app_namespace(&paths);
        assert!(report.completed);
        assert!(report.copied.is_empty());
        assert!(!paths.current_data.exists());
        assert!(paths.marker.is_file());
    }

    #[test]
    fn derives_current_and_legacy_config_and_data_siblings() {
        let paths = temp_paths("paths");
        assert_eq!(
            paths.current_config.file_name().unwrap(),
            CURRENT_APP_IDENTIFIER
        );
        assert_eq!(
            paths.current_data.file_name().unwrap(),
            CURRENT_APP_IDENTIFIER
        );
        assert_eq!(
            paths.legacy_config.file_name().unwrap(),
            LEGACY_APP_IDENTIFIER
        );
        assert_eq!(
            paths.legacy_data.file_name().unwrap(),
            LEGACY_APP_IDENTIFIER
        );
        assert_eq!(paths.marker.file_name().unwrap(), MIGRATION_MARKER_FILE);
    }

    #[test]
    fn active_migration_lock_is_not_stolen_and_unlock_checks_ownership() {
        let paths = temp_paths("lock");
        let lock = acquire_lock(&paths.current_config).unwrap();
        assert_eq!(
            acquire_lock(&paths.current_config).unwrap_err().kind(),
            io::ErrorKind::WouldBlock
        );
        fs::write(lock.path.join("owner"), "replacement-owner").unwrap();
        let lock_path = lock.path.clone();
        drop(lock);
        assert!(lock_path.exists());
        fs::remove_dir_all(lock_path).unwrap();
    }

    #[test]
    fn migration_failure_is_reported_without_panicking() {
        let paths = temp_paths("failure");
        write(&paths.current_config, "not a directory");
        let report = migrate_app_namespace(&paths);
        assert!(!report.completed);
        assert_eq!(report.issues.len(), 1);
    }

    #[test]
    fn existing_new_data_is_authoritative_and_partial_migration_recovers() {
        let paths = temp_paths("conflict");
        write(&paths.legacy_config.join(SETTINGS_FILE), "old");
        write(
            &paths.legacy_config.join("templates").join(TEMPLATE_FILE),
            "legacy templates",
        );
        write(&paths.current_config.join(SETTINGS_FILE), "new");

        let report = migrate_app_namespace(&paths);
        assert!(report.completed);
        assert_eq!(
            fs::read_to_string(paths.current_config.join(SETTINGS_FILE)).unwrap(),
            "new"
        );
        assert_eq!(report.conflicts, vec![SETTINGS_FILE]);
        assert_eq!(
            fs::read_to_string(paths.current_config.join("templates").join(TEMPLATE_FILE)).unwrap(),
            "legacy templates"
        );
    }

    #[test]
    fn corrupt_marker_is_recovered_without_overwriting_data() {
        let paths = temp_paths("marker");
        write(&paths.legacy_config.join(SETTINGS_FILE), "legacy");
        write(&paths.marker, "not json");
        let report = migrate_app_namespace(&paths);
        assert!(report.completed);
        assert_eq!(
            fs::read_to_string(paths.current_config.join(SETTINGS_FILE)).unwrap(),
            "legacy"
        );
        assert!(serde_json::from_str::<MigrationReport>(
            &fs::read_to_string(&paths.marker).unwrap()
        )
        .is_ok());
    }

    #[test]
    fn smoke_namespace_does_not_migrate_real_legacy_data() {
        let root = std::env::temp_dir().join(format!("voxly-smoke-{}", unique_suffix()));
        let paths = AppNamespacePaths::from_current(
            root.join("config/com.skybridgem.voxly.pr47.smoke"),
            root.join("data/com.skybridgem.voxly.pr47.smoke"),
        )
        .unwrap();
        write(&paths.legacy_config.join(SETTINGS_FILE), "legacy");
        let report = migrate_app_namespace(&paths);
        assert!(report.completed);
        assert!(!paths.current_config.join(SETTINGS_FILE).exists());
    }

    #[cfg(unix)]
    #[test]
    fn rejects_symlinked_legacy_file_and_templates_directory() {
        use std::os::unix::fs::symlink;
        let paths = temp_paths("symlink");
        let outside = paths.current_config.parent().unwrap().join("outside");
        write(&outside.join("settings.json"), "secret");
        fs::create_dir_all(&paths.legacy_config).unwrap();
        symlink(
            outside.join("settings.json"),
            paths.legacy_config.join(SETTINGS_FILE),
        )
        .unwrap();
        symlink(&outside, paths.legacy_config.join("templates")).unwrap();
        let report = migrate_app_namespace(&paths);
        assert_eq!(report.issues.len(), 2);
        assert!(!paths.current_config.join(SETTINGS_FILE).exists());
        assert!(!paths
            .current_config
            .join("templates")
            .join(TEMPLATE_FILE)
            .exists());
    }

    #[cfg(unix)]
    #[test]
    fn preserves_source_permissions() {
        use std::os::unix::fs::PermissionsExt;
        let paths = temp_paths("permissions");
        let source = paths.legacy_config.join(SETTINGS_FILE);
        write(&source, "settings");
        fs::set_permissions(&source, fs::Permissions::from_mode(0o600)).unwrap();
        let report = migrate_app_namespace(&paths);
        assert!(report.completed);
        let mode = fs::metadata(paths.current_config.join(SETTINGS_FILE))
            .unwrap()
            .permissions()
            .mode();
        assert_eq!(mode & 0o777, 0o600);
    }
}
