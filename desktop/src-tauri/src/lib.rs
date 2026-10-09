//! Latentic Agent Cloud for a person's own computer: a window on their organisation's server, an icon that says what their agents are
//! doing, and a notification when one finishes. The work is done by haas-helper, which the app starts and keeps going.

mod config;
mod folders;
mod helper;
mod links;
mod navigation;
mod notice;
mod signin;
mod tray;
mod updates;
mod window;

use tauri::{AppHandle, RunEvent};
use tauri_plugin_autostart::MacosLauncher;
use tauri_plugin_deep_link::DeepLinkExt;

/// Connects the app to a server: checks that it answers, keeps its address, starts the helper for it, and shows its pages.
#[tauri::command]
fn connect(app: AppHandle, server: String) -> Result<(), String> {
    let server = config::normalize(&server)?;
    config::reachable(&server)?;
    config::save(&app, &config::Config { server: Some(server.clone()) })?;
    helper::start(&app, &server);
    window::go(&app, &window::chat_address(&server));
    Ok(())
}

pub fn run() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _arguments, _directory| window::show(app)))
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_autostart::init(MacosLauncher::LaunchAgent, None))
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .manage(helper::Helper::default())
        .manage(tray::Pages::default())
        .manage(updates::Available::default())
        .manage(signin::Pending::default())
        .invoke_handler(tauri::generate_handler![connect])
        .setup(|app| {
            let handle = app.handle().clone();
            tray::build(&handle)?;
            updates::watch(handle.clone());
            if let Some(server) = config::load(&handle).server {
                helper::start(&handle, &server);
            }
            window::show(&handle);
            // For finding out why notifications do not appear on a computer: the system asks permission the first time, and a click on this one shows the window.
            if std::env::args().any(|argument| argument == "--test-notification") {
                let server = config::load(&handle).server.unwrap_or_default();
                notice::send(&handle, "Your agent finished", "This is a test of the notification", &server);
            }
            let linked = handle.clone();
            app.deep_link().on_open_url(move |event| {
                let server = config::load(&linked).server.unwrap_or_default();
                let urls = event.urls();
                if let Some(ticket) = urls.iter().find_map(|link| links::signed_in(link.as_str())) {
                    signin::complete(&linked, &ticket);
                } else if let Some(page) = urls.iter().find_map(|link| links::target(link.as_str(), &server)) {
                    window::go(&linked, &page);
                }
            });
            // A system that does not register the scheme when the app is installed is told here, which a development build needs.
            #[cfg(any(windows, target_os = "linux"))]
            let _ = app.deep_link().register_all();
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
