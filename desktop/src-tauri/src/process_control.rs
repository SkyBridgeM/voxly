//! Ownership helpers for the Go sidecar and the processes it starts.
//!
//! The desktop owns exactly the sidecar children it spawns. Stop requests are
//! cooperative first (a control command on the child's stdin), and only the
//! deadline expiry is forceful. ffmpeg is a child of the sidecar, not of this
//! process, so the sidecar is responsible for it during a graceful stop; the
//! forced path additionally targets the platform's process tree.

use std::process::{Child, Command};

/// Grace period between a cooperative stop request and a forced stop.
pub const GRACE_PERIOD: std::time::Duration = std::time::Duration::from_secs(8);

/// Put the child in its own process group, so a forced stop can reach the whole
/// tree (the sidecar plus the ffmpeg it spawned) instead of orphaning it.
#[cfg(unix)]
pub fn isolate_process_group(command: &mut Command) {
    use std::os::unix::process::CommandExt;
    command.process_group(0);
}

/// Windows has no equivalent to create here; the tree is stopped by process id.
#[cfg(windows)]
pub fn isolate_process_group(_command: &mut Command) {}

/// Forcefully stop the child and, where the platform allows it, its descendants.
///
/// Returns true when the process tree was stopped through the platform
/// mechanism, false when the direct child alone had to be killed. A failed tree
/// stop is not escalated further: the direct child is always killed, and a
/// descendant that outlives it finishes its current work and exits.
pub fn force_stop(child: &mut Child) -> bool {
    #[cfg(unix)]
    {
        let pid = child.id();
        // The child leads its own process group (see isolate_process_group), so
        // a negative pid addresses the whole group.
        let stopped = Command::new("kill")
            .arg("-KILL")
            .arg("--")
            .arg(format!("-{pid}"))
            .status()
            .map(|status| status.success())
            .unwrap_or(false);
        if stopped {
            let _ = child.wait();
            return true;
        }
    }
    #[cfg(windows)]
    {
        let pid = child.id();
        let stopped = Command::new("taskkill")
            .arg("/PID")
            .arg(pid.to_string())
            .arg("/T")
            .arg("/F")
            .status()
            .map(|status| status.success())
            .unwrap_or(false);
        if stopped {
            let _ = child.wait();
            return true;
        }
    }
    child.kill().is_ok()
}
