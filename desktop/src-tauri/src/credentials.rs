use std::collections::HashMap;
use std::fmt;
use std::path::Path;

use serde::Serialize;

const KEYRING_SERVICE: &str = "com.meeting-asr.desktop-experiment";

/// The only credential names accepted by the desktop credential boundary.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(crate) enum CredentialKey {
    MimoApiKey,
    DeepseekApiKey,
}

impl CredentialKey {
    pub(crate) fn parse(value: &str) -> Result<Self, CredentialError> {
        match value {
            "MIMO_API_KEY" => Ok(Self::MimoApiKey),
            "DEEPSEEK_API_KEY" => Ok(Self::DeepseekApiKey),
            _ => Err(CredentialError::InvalidKey),
        }
    }

    pub(crate) const fn env_name(self) -> &'static str {
        match self {
            Self::MimoApiKey => "MIMO_API_KEY",
            Self::DeepseekApiKey => "DEEPSEEK_API_KEY",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum CredentialOperation {
    Read,
    Save,
    Delete,
}

impl CredentialOperation {
    const fn as_str(self) -> &'static str {
        match self {
            Self::Read => "read",
            Self::Save => "save",
            Self::Delete => "delete",
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum CredentialError {
    InvalidKey,
    InvalidValue,
    SecureStoreUnavailable {
        key: CredentialKey,
        operation: CredentialOperation,
    },
    EnvironmentUnavailable,
}

impl fmt::Display for CredentialError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InvalidKey => formatter.write_str("Unsupported credential key."),
            Self::InvalidValue => formatter.write_str("Credential value cannot be empty."),
            Self::SecureStoreUnavailable { operation, key } => write!(
                formatter,
                "Secure credential storage is unavailable for {} during {}.",
                key.env_name(),
                operation.as_str()
            ),
            Self::EnvironmentUnavailable => {
                formatter.write_str("The development .env credential source is unavailable.")
            }
        }
    }
}

impl std::error::Error for CredentialError {}

/// Safe metadata returned to the frontend. It deliberately contains no value.
#[derive(Debug, Clone, Serialize, PartialEq, Eq)]
pub(crate) struct CredentialStatus {
    pub key: String,
    pub configured: bool,
    pub source: CredentialSource,
}

#[derive(Debug, Clone, Copy, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub(crate) enum CredentialSource {
    Session,
    SecureStore,
    Env,
    Missing,
}

pub(crate) struct ResolvedCredential {
    pub(crate) value: String,
    pub(crate) source: CredentialSource,
}

pub(crate) trait CredentialStore {
    fn get(&self, key: CredentialKey) -> Result<Option<String>, CredentialError>;
    fn save(&self, key: CredentialKey, value: &str) -> Result<(), CredentialError>;
    fn delete(&self, key: CredentialKey) -> Result<(), CredentialError>;
}

/// The platform-native store selected by keyring: macOS Keychain on macOS,
/// Windows Credential Manager on Windows, and Secret Service on Linux.
pub(crate) struct KeyringCredentialStore;

impl CredentialStore for KeyringCredentialStore {
    fn get(&self, key: CredentialKey) -> Result<Option<String>, CredentialError> {
        let entry = keyring::Entry::new(KEYRING_SERVICE, key.env_name()).map_err(|_| {
            CredentialError::SecureStoreUnavailable {
                key,
                operation: CredentialOperation::Read,
            }
        })?;

        match entry.get_password() {
            Ok(value) if !value.trim().is_empty() => Ok(Some(value)),
            Ok(_) | Err(keyring::Error::NoEntry) => Ok(None),
            Err(_) => Err(CredentialError::SecureStoreUnavailable {
                key,
                operation: CredentialOperation::Read,
            }),
        }
    }

    fn save(&self, key: CredentialKey, value: &str) -> Result<(), CredentialError> {
        if value.trim().is_empty() {
            return Err(CredentialError::InvalidValue);
        }
        let entry = keyring::Entry::new(KEYRING_SERVICE, key.env_name()).map_err(|_| {
            CredentialError::SecureStoreUnavailable {
                key,
                operation: CredentialOperation::Save,
            }
        })?;
        entry
            .set_password(value)
            .map_err(|_| CredentialError::SecureStoreUnavailable {
                key,
                operation: CredentialOperation::Save,
            })
    }

    fn delete(&self, key: CredentialKey) -> Result<(), CredentialError> {
        let entry = keyring::Entry::new(KEYRING_SERVICE, key.env_name()).map_err(|_| {
            CredentialError::SecureStoreUnavailable {
                key,
                operation: CredentialOperation::Delete,
            }
        })?;
        match entry.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(_) => Err(CredentialError::SecureStoreUnavailable {
                key,
                operation: CredentialOperation::Delete,
            }),
        }
    }
}

#[derive(Debug, Clone, Default)]
pub(crate) struct EnvironmentValues {
    values: HashMap<CredentialKey, String>,
}

impl EnvironmentValues {
    pub(crate) fn for_development(env_path: Option<&Path>) -> Result<Self, CredentialError> {
        let mut values = Self::default();
        values.read_process_environment();
        if let Some(path) = env_path {
            values.read_dotenv(path)?;
        }
        Ok(values)
    }

    fn read_process_environment(&mut self) {
        for key in [CredentialKey::MimoApiKey, CredentialKey::DeepseekApiKey] {
            if let Ok(value) = std::env::var(key.env_name()) {
                if !value.trim().is_empty() {
                    self.values.insert(key, value);
                }
            }
        }
    }

    fn read_dotenv(&mut self, path: &Path) -> Result<(), CredentialError> {
        let entries =
            dotenvy::from_path_iter(path).map_err(|_| CredentialError::EnvironmentUnavailable)?;
        for entry in entries {
            let (name, value) = entry.map_err(|_| CredentialError::EnvironmentUnavailable)?;
            let Ok(key) = CredentialKey::parse(&name) else {
                continue;
            };
            // Process environment values retain precedence over .env values,
            // matching the existing Python/Go development behavior.
            if !self.values.contains_key(&key) && !value.trim().is_empty() {
                self.values.insert(key, value);
            }
        }
        Ok(())
    }

    fn get(&self, key: CredentialKey) -> Option<&str> {
        self.values.get(&key).map(String::as_str)
    }
}

pub(crate) fn resolve_credential<S: CredentialStore>(
    store: &S,
    key: CredentialKey,
    session_value: Option<&str>,
    runtime_mode: crate::RuntimeMode,
    environment: &EnvironmentValues,
) -> Result<Option<ResolvedCredential>, CredentialError> {
    if let Some(value) = session_value.filter(|value| !value.trim().is_empty()) {
        return Ok(Some(ResolvedCredential {
            value: value.trim().to_string(),
            source: CredentialSource::Session,
        }));
    }

    match store.get(key) {
        Ok(Some(value)) if !value.trim().is_empty() => {
            return Ok(Some(ResolvedCredential {
                value: value.trim().to_string(),
                source: CredentialSource::SecureStore,
            }));
        }
        Ok(Some(_)) => {}
        Ok(None) => {}
        Err(error) if runtime_mode == crate::RuntimeMode::Development => {
            // Development may retain the existing .env compatibility path if
            // the OS store is unavailable. Save/delete never use plaintext.
            if let Some(value) = environment.get(key) {
                return Ok(Some(ResolvedCredential {
                    value: value.trim().to_string(),
                    source: CredentialSource::Env,
                }));
            }
            return Err(error);
        }
        Err(error) => return Err(error),
    }

    if runtime_mode == crate::RuntimeMode::Development {
        if let Some(value) = environment.get(key) {
            return Ok(Some(ResolvedCredential {
                value: value.trim().to_string(),
                source: CredentialSource::Env,
            }));
        }
    }

    Ok(None)
}

pub(crate) fn status_for<S: CredentialStore>(
    store: &S,
    key: CredentialKey,
    session_value: Option<&str>,
    runtime_mode: crate::RuntimeMode,
    environment: &EnvironmentValues,
) -> Result<CredentialStatus, CredentialError> {
    let resolved = resolve_credential(store, key, session_value, runtime_mode, environment)?;
    Ok(match resolved {
        Some(resolved) => CredentialStatus {
            key: key.env_name().to_string(),
            configured: true,
            source: resolved.source,
        },
        None => CredentialStatus {
            key: key.env_name().to_string(),
            configured: false,
            source: CredentialSource::Missing,
        },
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::sync::Mutex;

    #[derive(Default)]
    struct MemoryCredentialStore {
        values: Mutex<HashMap<CredentialKey, String>>,
        unavailable: bool,
    }

    impl MemoryCredentialStore {
        fn unavailable() -> Self {
            Self {
                unavailable: true,
                ..Default::default()
            }
        }
    }

    impl CredentialStore for MemoryCredentialStore {
        fn get(&self, key: CredentialKey) -> Result<Option<String>, CredentialError> {
            if self.unavailable {
                return Err(CredentialError::SecureStoreUnavailable {
                    key,
                    operation: CredentialOperation::Read,
                });
            }
            Ok(self.values.lock().unwrap().get(&key).cloned())
        }

        fn save(&self, key: CredentialKey, value: &str) -> Result<(), CredentialError> {
            if self.unavailable {
                return Err(CredentialError::SecureStoreUnavailable {
                    key,
                    operation: CredentialOperation::Save,
                });
            }
            self.values.lock().unwrap().insert(key, value.to_string());
            Ok(())
        }

        fn delete(&self, key: CredentialKey) -> Result<(), CredentialError> {
            if self.unavailable {
                return Err(CredentialError::SecureStoreUnavailable {
                    key,
                    operation: CredentialOperation::Delete,
                });
            }
            self.values.lock().unwrap().remove(&key);
            Ok(())
        }
    }

    #[test]
    fn status_exposes_only_safe_metadata() {
        let store = MemoryCredentialStore::default();
        let environment = EnvironmentValues::default();
        let missing = status_for(
            &store,
            CredentialKey::MimoApiKey,
            None,
            crate::RuntimeMode::Development,
            &environment,
        )
        .unwrap();
        assert!(!missing.configured);
        assert_eq!(missing.source, CredentialSource::Missing);

        store
            .save(CredentialKey::MimoApiKey, "test-secret")
            .unwrap();
        let configured = status_for(
            &store,
            CredentialKey::MimoApiKey,
            None,
            crate::RuntimeMode::Development,
            &environment,
        )
        .unwrap();
        let json = serde_json::to_string(&configured).unwrap();
        assert!(configured.configured);
        assert_eq!(configured.source, CredentialSource::SecureStore);
        assert!(!json.contains("test-secret"));
    }

    #[test]
    fn resolution_order_is_session_then_secure_store_then_env() {
        let store = MemoryCredentialStore::default();
        store
            .save(CredentialKey::MimoApiKey, "secure-secret")
            .unwrap();
        let mut environment = EnvironmentValues::default();
        environment
            .values
            .insert(CredentialKey::MimoApiKey, "env-secret".to_string());

        let session = resolve_credential(
            &store,
            CredentialKey::MimoApiKey,
            Some("session-secret"),
            crate::RuntimeMode::Development,
            &environment,
        )
        .unwrap()
        .unwrap();
        assert_eq!(session.value, "session-secret");
        assert_eq!(session.source, CredentialSource::Session);

        let secure = resolve_credential(
            &store,
            CredentialKey::MimoApiKey,
            None,
            crate::RuntimeMode::Development,
            &environment,
        )
        .unwrap()
        .unwrap();
        assert_eq!(secure.value, "secure-secret");
        assert_eq!(secure.source, CredentialSource::SecureStore);

        store.delete(CredentialKey::MimoApiKey).unwrap();
        let env = resolve_credential(
            &store,
            CredentialKey::MimoApiKey,
            None,
            crate::RuntimeMode::Development,
            &environment,
        )
        .unwrap()
        .unwrap();
        assert_eq!(env.value, "env-secret");
        assert_eq!(env.source, CredentialSource::Env);
    }

    #[test]
    fn production_does_not_resolve_development_environment() {
        let store = MemoryCredentialStore::default();
        let mut environment = EnvironmentValues::default();
        environment
            .values
            .insert(CredentialKey::MimoApiKey, "env-secret".to_string());

        let resolved = resolve_credential(
            &store,
            CredentialKey::MimoApiKey,
            None,
            crate::RuntimeMode::Production,
            &environment,
        )
        .unwrap();
        assert!(resolved.is_none());
    }

    #[test]
    fn development_reads_dotenv_without_exposing_values() {
        let path = std::env::temp_dir().join(format!(
            "meeting-asr-credentials-{}.env",
            std::process::id()
        ));
        fs::write(
            &path,
            "MIMO_API_KEY=dotenv-secret\nDEEPSEEK_API_KEY=deepseek-secret\n",
        )
        .unwrap();
        let environment = EnvironmentValues::default();
        let mut from_file = environment.clone();
        from_file.read_dotenv(&path).unwrap();
        fs::remove_file(&path).ok();

        assert_eq!(
            from_file.get(CredentialKey::MimoApiKey),
            Some("dotenv-secret")
        );
        assert_eq!(
            from_file.get(CredentialKey::DeepseekApiKey),
            Some("deepseek-secret")
        );
    }

    #[test]
    fn unavailable_store_is_not_replaced_by_plaintext_save() {
        let store = MemoryCredentialStore::unavailable();
        let result = store.save(CredentialKey::MimoApiKey, "secret");
        assert!(matches!(
            result,
            Err(CredentialError::SecureStoreUnavailable {
                operation: CredentialOperation::Save,
                ..
            })
        ));
        assert!(!format!("{result:?}").contains("secret"));
    }
}
