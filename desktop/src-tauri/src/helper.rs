//! The helper, which does the work: it signs in, keeps the folders, bridges editors and follows the person's turns. The app starts it,
//! keeps it going, and asks it, over the address only this computer reaches, what it knows.

use serde::Deserialize;
use std::{
    fs,
    path::PathBuf,
    process::{Child, Command, Stdio},
    sync::{
        atomic::{AtomicU64, Ordering},
        Mutex,
    },
    time::Duration,
};
use tauri::{AppHandle, Manager};

#[derive(Default)]
pub struct Helper {
    child: Mutex<Option<Child>>,
    generation: AtomicU64,
}

#[derive(Deserialize, Clone, Debug, Default, PartialEq)]
#[serde(default)]
pub struct Status {
    pub server: String,
    #[serde(rename = "signedIn")]
    pub signed_in: bool,
    pub person: String,
    pub connected: bool,
    pub tray: String,
}

#[derive(Deserialize, Clone, Debug, PartialEq)]
#[serde(default)]
pub struct Turn {
    pub job: String,
    pub instance: String,
    pub thread: String,
    pub brief: String,
    pub state: String,
}

impl Default for Turn {
    fn default() -> Self {
        Turn { job: String::new(), instance: String::new(), thread: String::new(), brief: String::new(), state: String::new() }
    }
}

#[derive(Deserialize)]
struct Local {
    address: String,
    token: String,
}

/// Where the helper keeps its settings and says how to reach it: the same folder the helper itself uses, which is the system's own
/// folder for a program's settings and a folder named gusto in it.
fn settings_dir(app: &AppHandle) -> Option<PathBuf> {
    app.path().config_dir().ok().map(|dir| dir.join("gusto"))
}

fn local(app: &AppHandle) -> Option<Local> {
    let text = fs::read_to_string(settings_dir(app)?.join("helper.json")).ok()?;
    serde_json::from_str(&text).ok()
}

fn ask<T: for<'de> Deserialize<'de>>(app: &AppHandle, route: &str) -> Option<T> {
    let local = local(app)?;
    let agent = ureq::AgentBuilder::new().timeout(Duration::from_secs(3)).build();
    agent
        .get(&format!("http://{}{}", local.address, route))
        .set("Authorization", &format!("Bearer {}", local.token))
        .call()
        .ok()?
        .into_json()
        .ok()
}

/// Asks the helper to keep a folder on this computer the same as the agent's folder in the cloud that is started for it. The helper
/// says why when it will not, and that is what the person is told.
pub fn keep_folder(app: &AppHandle, key: &str, path: &str) -> Result<(), String> {
    let local = local(app).ok_or("the helper is not running")?;
    let agent = ureq::AgentBuilder::new().timeout(Duration::from_secs(5)).build();
    agent
        .post(&format!("http://{}/v1/folders", local.address))
        .set("Authorization", &format!("Bearer {}", local.token))
        .send_json(serde_json::json!({ "key": key, "path": path }))
        .map(|_| ())
        .map_err(|error| match error {
            ureq::Error::Status(_, response) => response.into_string().unwrap_or_default().trim().to_string(),
            other => other.to_string(),
        })
}

pub fn status(app: &AppHandle) -> Option<Status> {
    ask(app, "/v1/status")
}

pub fn turns(app: &AppHandle) -> Option<Vec<Turn>> {
    ask(app, "/v1/turns")
}

/// The program the app carries: it is in the folder the app's own program is in.
fn program() -> PathBuf {
    let name = if cfg!(windows) { "gusto-helper.exe" } else { "gusto-helper" };
    std::env::current_exe().ok().and_then(|exe| exe.parent().map(|dir| dir.join(name))).unwrap_or_else(|| PathBuf::from(name))
}

fn log(app: &AppHandle) -> Stdio {
    app.path()
        .app_log_dir()
        .ok()
        .and_then(|dir| fs::create_dir_all(&dir).ok().map(|_| dir.join("helper.log")))
        .and_then(|file| fs::OpenOptions::new().create(true).append(true).open(file).ok())
        .map(Stdio::from)
        .unwrap_or_else(Stdio::null)
}

/// Starts the helper for a server, and starts it again when it ends, until the app tells it to stop or follow another server. A helper
/// already running, which the person started themselves or at login, is used and not started over.
pub fn start(app: &AppHandle, server: &str) {
    let helper = app.state::<Helper>();
    let generation = helper.generation.fetch_add(1, Ordering::SeqCst) + 1;
    kill(&helper);
    let app = app.clone();
    let server = server.to_string();
    std::thread::spawn(move || loop {
        let helper = app.state::<Helper>();
        if helper.generation.load(Ordering::SeqCst) != generation {
            return;
        }
        if status(&app).is_none() {
            let spawned = Command::new(program())
                .args(["run", "-server", &server, "-tray=false"])
                .stdin(Stdio::null())
                .stdout(log(&app))
                .stderr(log(&app))
                .spawn();
            match spawned {
                Ok(child) => {
                    *helper.child.lock().unwrap() = Some(child);
                }
                Err(error) => eprintln!("the helper could not be started: {error}"),
            }
        }
        std::thread::sleep(Duration::from_secs(5));
    });
}

fn kill(helper: &Helper) {
    if let Some(mut child) = helper.child.lock().unwrap().take() {
        let _ = child.kill();
        let _ = child.wait();
    }
}

/// Ends the helper the app started.
pub fn stop(app: &AppHandle) {
    let helper = app.state::<Helper>();
    helper.generation.fetch_add(1, Ordering::SeqCst);
    kill(&helper);
}

/// Signs this computer in as the person, which opens their browser to approve it.
pub fn sign_in(server: &str) {
    let _ = Command::new(program()).args(["login", "-server", server]).stdin(Stdio::null()).stdout(Stdio::null()).stderr(Stdio::null()).spawn();
}

/// The address of the page that shows a turn: its conversation, or the job when it belongs to none.
pub fn page_of(server: &str, turn: &Turn) -> String {
    let server = server.trim_end_matches('/');
    if !turn.instance.is_empty() && !turn.thread.is_empty() {
        format!("{server}/instances/{}/threads/{}", turn.instance, turn.thread)
    } else {
        format!("{server}/jobs/{}", turn.job)
    }
}


#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_turn_in_a_conversation_opens_the_conversation_and_any_other_opens_the_job() {
        let in_thread = Turn { job: "j1".into(), instance: "i1".into(), thread: "t1".into(), ..Turn::default() };
        assert_eq!(page_of("https://s/", &in_thread), "https://s/instances/i1/threads/t1");
        let alone = Turn { job: "j9".into(), ..Turn::default() };
        assert_eq!(page_of("https://s", &alone), "https://s/jobs/j9");
    }

    #[test]
    fn what_the_helper_says_is_read_and_a_missing_field_is_empty() {
        let status: Status = serde_json::from_str(r#"{"server":"https://s","signedIn":true,"person":"ada","connected":true}"#).unwrap();
        assert!(status.signed_in && status.connected && status.person == "ada" && status.tray.is_empty());
        let turns: Vec<Turn> = serde_json::from_str(r#"[{"job":"a","brief":"b","state":"running","at":5}]"#).unwrap();
        assert_eq!(turns[0].state, "running");
    }
}
