//! What the app keeps for itself: which server it is connected to.

use serde::{Deserialize, Serialize};
use std::{fs, path::PathBuf, time::Duration};
use tauri::{AppHandle, Manager};

#[derive(Serialize, Deserialize, Default, Clone, PartialEq, Debug)]
pub struct Config {
    pub server: Option<String>,
}

fn path(app: &AppHandle) -> Option<PathBuf> {
    app.path().app_config_dir().ok().map(|dir| dir.join("config.json"))
}

pub fn load(app: &AppHandle) -> Config {
    path(app)
        .and_then(|file| fs::read_to_string(file).ok())
        .and_then(|text| serde_json::from_str(&text).ok())
        .unwrap_or_default()
}

pub fn save(app: &AppHandle, config: &Config) -> Result<(), String> {
    let file = path(app).ok_or("the app has nowhere to keep its settings")?;
    if let Some(dir) = file.parent() {
        fs::create_dir_all(dir).map_err(|e| e.to_string())?;
    }
    let text = serde_json::to_string_pretty(config).map_err(|e| e.to_string())?;
    fs::write(file, text).map_err(|e| e.to_string())
}

/// The address of a server as the app keeps it: with its scheme, no user name or password, no path, and no slash at the end. A bare
/// name is taken to be https, since a server that people sign in to is one.
pub fn normalize(typed: &str) -> Result<String, String> {
    let typed = typed.trim();
    if typed.is_empty() {
        return Err("type the address of your organization's server".into());
    }
    let with_scheme = if typed.contains("://") { typed.to_string() } else { format!("https://{typed}") };
    let parsed = url::Url::parse(&with_scheme).map_err(|_| format!("{typed} is not an address"))?;
    if parsed.scheme() != "https" && parsed.scheme() != "http" {
        return Err("the address must be http or https".into());
    }
    if !parsed.username().is_empty() || parsed.password().is_some() {
        return Err("the address must not hold a user name or a password".into());
    }
    let host = parsed.host_str().ok_or("the address has no host")?;
    let local = host == "localhost" || host == "127.0.0.1" || host == "[::1]" || host == "::1";
    if parsed.scheme() == "http" && !local {
        return Err("the address must be https, since it is not this computer".into());
    }
    let mut out = format!("{}://{}", parsed.scheme(), host);
    if let Some(port) = parsed.port() {
        out.push_str(&format!(":{port}"));
    }
    Ok(out)
}

/// Whether something answers at the address as a server of this product does.
pub fn reachable(server: &str) -> Result<(), String> {
    let agent = ureq::AgentBuilder::new().timeout(Duration::from_secs(6)).redirects(0).build();
    match agent.get(&format!("{server}/healthz")).call() {
        Ok(answer) if answer.status() == 200 => Ok(()),
        Ok(answer) => Err(format!("{server} answered {}, and a server of this product answers 200 at /healthz", answer.status())),
        Err(ureq::Error::Status(code, _)) => Err(format!("{server} answered {code}, and a server of this product answers 200 at /healthz")),
        Err(error) => Err(format!("{server} cannot be reached: {error}")),
    }
}

#[cfg(test)]
mod tests {
    use super::normalize;

    #[test]
    fn an_address_is_kept_as_a_scheme_a_host_and_a_port() {
        for (typed, want) in [
            ("gusto.example.com", "https://gusto.example.com"),
            ("  https://gusto.example.com/  ", "https://gusto.example.com"),
            ("https://gusto.example.com/some/path?x=1#y", "https://gusto.example.com"),
            ("http://localhost:8040", "http://localhost:8040"),
            ("http://127.0.0.1:8040/", "http://127.0.0.1:8040"),
            ("https://gusto.example.com:8443", "https://gusto.example.com:8443"),
        ] {
            assert_eq!(normalize(typed).as_deref(), Ok(want), "{typed}");
        }
    }

    #[test]
    fn an_address_that_is_not_one_a_person_signs_in_to_is_refused() {
        for typed in ["", "   ", "ftp://example.com", "http://example.com", "https://user:pass@example.com", "https://", "not an address"] {
            assert!(normalize(typed).is_err(), "{typed:?} was taken");
        }
    }
}
