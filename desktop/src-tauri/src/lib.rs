//! Latentic Agent Cloud for a person's own computer: a window on their organisation's server, an icon that says what their agents are
//! doing, and a notification when one finishes. The work is done by haas-helper, which the app starts and keeps going.

mod config;
mod helper;
mod tray;
mod window;

use tauri::{AppHandle, RunEvent};

/// Connects the app to a server: checks that it answers, keeps its address, starts the helper for it, and shows its pages.
#[tauri::command]
fn connect(app: AppHandle, server: String) -> Result<(), String> {
    let server = config::normalize(&server)?;
    config::reachable(&server)?;
    config::save(&app, &config::Config { server: Some(server.clone()) })?;
    helper::start(&app, &server);
    window::go(&app, &server);
    Ok(())
}

pub fn run() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _arguments, _directory| window::show(app)))
        .plugin(tauri_plugin_opener::init())
        .manage(helper::Helper::default())
        .manage(tray::Pages::default())
        .invoke_handler(tauri::generate_handler![connect])
        .setup(|app| {
            let handle = app.handle().clone();
            tray::build(&handle)?;
            if let Some(server) = config::load(&handle).server {
                helper::start(&handle, &server);
            }
            window::show(&handle);
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("the app could not be built");
    app.run(|app, event| match event {
        // The last window closing does not end the app: its icon stays, so that the person is told when a turn ends.
        RunEvent::ExitRequested { api, code, .. } if code.is_none() => api.prevent_exit(),
        RunEvent::Exit => helper::stop(app),
        #[cfg(target_os = "macos")]
        RunEvent::Reopen { .. } => window::show(app),
        _ => {}
    });
}
