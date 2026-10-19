//! The icon, its menu, and the notification when a turn ends. What is in them comes from the helper, which follows the person's turns.

use crate::{config, helper, local, notice, updates, window};
use std::{
    collections::HashMap,
    sync::Mutex,
    time::Duration,
};
use tauri::{
    menu::{CheckMenuItemBuilder, Menu, MenuBuilder, MenuItemBuilder},
    tray::TrayIconBuilder,
    AppHandle, Manager,
};
use tauri_plugin_autostart::ManagerExt;

const SLOTS: usize = 8;
const ID: &str = "latentic";

/// The pages the menu's turns open, by the id of the menu item.
#[derive(Default)]
pub struct Pages(Mutex<HashMap<String, String>>);

/// What the menu says, kept to tell whether it changed.
#[derive(Clone, PartialEq, Debug)]
struct View {
    standing: String,
    here: bool,
    turns: Vec<(String, String)>,
    sign_in: bool,
    login: bool,
    update: Option<String>,
}

fn word(state: &str) -> &str {
    match state {
        "queued" => "waiting",
        "running" => "working",
        "passed" => "finished",
        "failed" => "failed",
        "interrupted" => "stopped",
        other => other,
    }
}

fn ended(state: &str) -> bool {
    matches!(state, "passed" | "failed" | "interrupted")
}

fn short(brief: &str) -> String {
    let line = brief.split_whitespace().collect::<Vec<_>>().join(" ");
    if line.is_empty() {
        return "a turn".into();
    }
    if line.chars().count() <= 48 {
        return line;
    }
    format!("{}…", line.chars().take(47).collect::<String>())
}

fn standing(status: &Option<helper::Status>) -> String {
    match status {
        None => "Starting…".into(),
        Some(s) if !s.signed_in => "Not signed in".into(),
        Some(s) if !s.connected => "Reconnecting…".into(),
        Some(s) => format!("Signed in as {}", s.person),
    }
}

/// The turns the menu lists: those still going first, then those that ended, newest first as the helper gives them.
fn listed(turns: &[helper::Turn], server: &str) -> Vec<(String, String)> {
    let going = turns.iter().filter(|t| !ended(&t.state));
    let done = turns.iter().filter(|t| ended(&t.state));
    going
        .chain(done)
        .take(SLOTS)
        .map(|t| (format!("{} — {}", short(&t.brief), word(&t.state)), helper::page_of(server, t)))
        .collect()
}

fn menu(app: &AppHandle, view: &View) -> tauri::Result<Menu<tauri::Wry>> {
    let mut builder = MenuBuilder::new(app);
    builder = builder.item(&MenuItemBuilder::with_id("standing", &view.standing).enabled(false).build(app)?);
    let state = app.state::<Pages>();
    let mut pages = state.0.lock().unwrap();
    pages.clear();
    for (i, (label, page)) in view.turns.iter().enumerate() {
        let id = format!("turn:{i}");
        pages.insert(id.clone(), page.clone());
        builder = builder.item(&MenuItemBuilder::with_id(id, label).build(app)?);
    }
    builder = builder.separator().item(&MenuItemBuilder::with_id("open", "Open gusto").build(app)?);
    if view.sign_in {
        builder = builder.item(&MenuItemBuilder::with_id("signin", "Sign in…").build(app)?);
    }
    if let Some(version) = &view.update {
        builder = builder.item(&MenuItemBuilder::with_id("update", format!("Update to {version} and restart")).build(app)?);
    }
    builder
        .item(&CheckMenuItemBuilder::with_id("login", "Start at login").checked(view.login).build(app)?)
        .item(&CheckMenuItemBuilder::with_id("here", "On this computer").checked(view.here).build(app)?)
        .item(&MenuItemBuilder::with_id("server", if view.here { "Connect to an organization…" } else { "Change organization…" }).build(app)?)
        .separator()
        .item(&MenuItemBuilder::with_id("quit", "Quit").build(app)?)
        .build()
}

/// Whether the app starts when the person signs in to the computer.
fn login(app: &AppHandle) -> bool {
    app.autolaunch().is_enabled().unwrap_or(false)
}

pub fn build(app: &AppHandle) -> tauri::Result<()> {
    let here = local::on_this_computer(app);
    let empty = menu(app, &View { standing: "Starting…".into(), here, turns: vec![], sign_in: false, login: login(app), update: updates::found(app) })?;
    TrayIconBuilder::with_id(ID)
        .icon(tauri::include_image!("icons/tray.png"))
        .icon_as_template(true)
        .tooltip("gusto")
        .menu(&empty)
        .on_menu_event(|app, event| {
            let id = event.id().as_ref().to_string();
            match id.as_str() {
                "open" => window::show(app),
                "server" => window::ask_for_server(app),
                "here" => {
                    if !local::on_this_computer(app) {
                        let _ = config::save(app, &config::Config { server: None });
                        helper::stop(app);
                        window::start_here(app);
                    }
                }
                "update" => updates::install(app),
                "login" => {
                    let launch = app.autolaunch();
                    let _ = if launch.is_enabled().unwrap_or(false) { launch.disable() } else { launch.enable() };
                }
                "signin" => {
                    if let Some(server) = config::load(app).server {
                        helper::sign_in(&server);
                    }
                }
                "quit" => app.exit(0),
                other => {
                    let page = app.state::<Pages>().0.lock().unwrap().get(other).cloned();
                    if let Some(page) = page {
                        window::go(app, &page);
                    }
                }
            }
        })
        .build(app)?;
    watch(app.clone());
    Ok(())
}

/// Looks at what the helper says every few seconds: redraws the menu when it changed, and says when a turn that was going has ended.
fn watch(app: AppHandle) {
    std::thread::spawn(move || {
        let mut last: Option<View> = None;
        let mut going: HashMap<String, bool> = HashMap::new();
        let mut first = true;
        loop {
            std::thread::sleep(Duration::from_secs(3));
            let here = local::on_this_computer(&app);
            let server = local::server(&app).unwrap_or_default();
            let status = if here { None } else { helper::status(&app) };
            let turns = if here { vec![] } else { helper::turns(&app).unwrap_or_default() };
            let view = View {
                standing: if here { "On this computer".into() } else { standing(&status) },
                here,
                turns: listed(&turns, &server),
                sign_in: !here && status.as_ref().map(|s| !s.signed_in).unwrap_or(false) && !server.is_empty(),
                login: login(&app),
                update: updates::found(&app),
            };
            if last.as_ref() != Some(&view) {
                if let (Some(tray), Ok(built)) = (app.tray_by_id(ID), menu(&app, &view)) {
                    let _ = tray.set_menu(Some(built));
                }
                last = Some(view);
            }
            for turn in &turns {
                let was_going = going.get(&turn.job).copied();
                going.insert(turn.job.clone(), !ended(&turn.state));
                // A turn the helper told of for the first time already ended, which is what a reconnect or a start repeats, is not news.
                if !first && was_going == Some(true) && ended(&turn.state) {
                    notice::send(&app, &format!("Your agent {}", word(&turn.state)), &short(&turn.brief), &helper::page_of(&server, turn));
                }
            }
            first = false;
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    fn turn(job: &str, state: &str) -> helper::Turn {
        helper::Turn { job: job.into(), instance: "i".into(), thread: "t".into(), brief: "write the report".into(), state: state.into() }
    }

    #[test]
    fn running_turns_come_before_ended_ones_and_the_list_is_bounded() {
        let mut turns = vec![turn("a", "passed"), turn("b", "running"), turn("c", "queued")];
        for i in 0..20 {
            turns.push(turn(&format!("x{i}"), "passed"));
        }
        let lines = listed(&turns, "https://s");
        assert_eq!(lines.len(), SLOTS);
        assert_eq!(lines[0].0, "write the report — working");
        assert_eq!(lines[1].0, "write the report — waiting");
        assert_eq!(lines[2].0, "write the report — finished");
        assert_eq!(lines[0].1, "https://s/instances/i/threads/t");
    }

    #[test]
    fn a_long_or_empty_brief_fits_on_one_line() {
        assert_eq!(short("  \n "), "a turn");
        let long = short(&"word ".repeat(40));
        assert_eq!(long.chars().count(), 48);
        assert!(long.ends_with('…') && !long.contains('\n'));
    }

    #[test]
    fn the_menu_says_how_the_helper_stands() {
        assert_eq!(standing(&None), "Starting…");
        let mut status = helper::Status::default();
        assert_eq!(standing(&Some(status.clone())), "Not signed in");
        status.signed_in = true;
        assert_eq!(standing(&Some(status.clone())), "Reconnecting…");
        status.connected = true;
        status.person = "ada".into();
        assert_eq!(standing(&Some(status)), "Signed in as ada");
    }
}
