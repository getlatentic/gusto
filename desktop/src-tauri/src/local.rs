//! haas on this computer, for a person with no organisation: the app uses the one already running, which the install script left as a
//! service, or starts the one it carries and stops it when the app quits. It signs the window in with a link haas makes for it, so the
//! app never holds haas's secret.

use crate::config;
use std::{
    fs,
    path::PathBuf,
    process::{Child, Command, Stdio},
    sync::Mutex,
    time::{Duration, Instant},
};
use tauri::{AppHandle, Manager};

#[derive(Default)]
pub struct Local {
    child: Mutex<Option<Child>>,
    address: Mutex<Option<String>>,
}

/// The haas the app carries, beside its own program.
fn program() -> PathBuf {
    let name = if cfg!(windows) { "haas.exe" } else { "haas" };
    std::env::current_exe().ok().and_then(|exe| exe.parent().map(|dir| dir.join(name))).unwrap_or_else(|| PathBuf::from(name))
}

fn quiet(command: &mut Command) -> &mut Command {
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        // No console window for a program the app runs.
        command.creation_flags(0x0800_0000);
    }
    command
}

/// A new link that signs in to haas on this computer, when it answers.
fn link() -> Result<String, String> {
    let out = quiet(Command::new(program()).args(["local", "link"]).stdin(Stdio::null()))
        .output()
        .map_err(|error| format!("haas could not be run: {error}"))?;
    let said = String::from_utf8_lossy(&out.stdout).trim().to_string();
    if out.status.success() && said.starts_with("http://127.0.0.1:") {
        return Ok(said);
    }
    Err(String::from_utf8_lossy(&out.stderr).trim().to_string())
}

/// Where a link signs in to: its scheme, host and port.
pub fn origin(link: &str) -> Option<String> {
    let parsed = url::Url::parse(link).ok()?;
    Some(format!("{}://{}:{}", parsed.scheme(), parsed.host_str()?, parsed.port()?))
}

fn log(app: &AppHandle) -> Stdio {
    app.path()
        .app_log_dir()
        .ok()
        .and_then(|dir| fs::create_dir_all(&dir).ok().map(|_| dir.join("haas.log")))
        .and_then(|file| fs::OpenOptions::new().create(true).append(true).open(file).ok())
        .map(Stdio::from)
        .unwrap_or_else(Stdio::null)
}

/// Finds haas on this computer, or starts the one the app carries, and is a link that signs in to it.
pub fn start(app: &AppHandle) -> Result<String, String> {
    let local = app.state::<Local>();
    let found = link().or_else(|_| {
        let child = quiet(Command::new(program()).args(["local", "run"]).stdin(Stdio::null()).stdout(log(app)).stderr(log(app)))
            .spawn()
            .map_err(|error| format!("haas could not be started: {error}"))?;
        *local.child.lock().unwrap() = Some(child);
        let deadline = Instant::now() + Duration::from_secs(30);
        loop {
            std::thread::sleep(Duration::from_millis(250));
            match link() {
                Ok(found) => return Ok(found),
                Err(error) if Instant::now() > deadline => return Err(format!("haas did not start: {error}")),
                Err(_) => {}
            }
        }
    })?;
    *local.address.lock().unwrap() = origin(&found);
    Ok(found)
}

/// A new sign-in link for the haas the app is on, for when its session ended.
pub fn sign_in_again() -> Result<String, String> {
    link()
}

/// Ends the haas the app started. One the install script left as a service keeps running.
pub fn stop(app: &AppHandle) {
    let local = app.state::<Local>();
    if let Some(mut child) = local.child.lock().unwrap().take() {
        let _ = child.kill();
        let _ = child.wait();
    }
    *local.address.lock().unwrap() = None;
}

/// Whether the app is on this computer, which is when no organisation's server is set.
pub fn on_this_computer(app: &AppHandle) -> bool {
    config::load(app).server.is_none()
}

/// The server the window shows: the organisation's, or haas on this computer once it answers.
pub fn server(app: &AppHandle) -> Option<String> {
    config::load(app).server.or_else(|| app.state::<Local>().address.lock().unwrap().clone())
}

#[cfg(test)]
mod tests {
    use super::origin;

    #[test]
    fn a_link_signs_in_to_its_own_scheme_host_and_port() {
        assert_eq!(origin("http://127.0.0.1:7140/local/enter?n=a&e=1&c=b").as_deref(), Some("http://127.0.0.1:7140"));
        assert_eq!(origin("not a link"), None);
    }
}
