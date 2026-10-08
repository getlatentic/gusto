//! The window: the person's own server, in the app, or the page that asks for its address when there is none.

use crate::config;
use tauri::{AppHandle, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder, WindowEvent};

const LABEL: &str = "main";

fn main_window(app: &AppHandle) -> Option<WebviewWindow> {
    app.get_webview_window(LABEL)
}

fn build(app: &AppHandle, target: WebviewUrl) -> tauri::Result<WebviewWindow> {
    let window = WebviewWindowBuilder::new(app, LABEL, target)
        .title("Latentic Agent Cloud")
        .inner_size(1200.0, 820.0)
        .min_inner_size(640.0, 480.0)
        .build()?;
    // Closing the window leaves the app running, with its icon, so that the person is still told when a turn ends. Quit is in the menu.
    let hidden = window.clone();
    window.on_window_event(move |event| {
        if let WindowEvent::CloseRequested { api, .. } = event {
            api.prevent_close();
            let _ = hidden.hide();
        }
    });
    Ok(window)
}

/// Shows the window: the server's pages when the app is connected to one, and the page that asks for its address when it is not.
pub fn show(app: &AppHandle) {
    if let Some(window) = main_window(app) {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
        return;
    }
    let target = match config::load(app).server.and_then(|server| server.parse().ok()) {
        Some(server) => WebviewUrl::External(server),
        None => WebviewUrl::App("index.html".into()),
    };
    if let Ok(window) = build(app, target) {
        let _ = window.set_focus();
    }
}

/// Shows an address of the server in the window, opening it if it is not open.
pub fn go(app: &AppHandle, address: &str) {
    show(app);
    if let (Some(window), Ok(address)) = (main_window(app), address.parse()) {
        let _ = window.navigate(address);
    }
}

/// Shows the page that asks for the server's address, which is how the person changes it.
pub fn ask_for_server(app: &AppHandle) {
    if let Some(window) = main_window(app) {
        let _ = window.destroy();
    }
    if let Ok(window) = build(app, WebviewUrl::App("index.html".into())) {
        let _ = window.set_focus();
    }
}
