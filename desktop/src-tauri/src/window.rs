//! The window: the person's own server, in the app, or the page that asks for its address when there is none.

use crate::{folders, local, navigation, navigation::Move, signin};
use tauri::{AppHandle, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder, WindowEvent};
use tauri_plugin_opener::OpenerExt;

const LABEL: &str = "main";

fn main_window(app: &AppHandle) -> Option<WebviewWindow> {
    app.get_webview_window(LABEL)
}

/// Decides each address the window is about to show. What changes the window is done after this answers, not inside it, which the
/// system's web view does not take well.
fn allowed(app: AppHandle) -> impl Fn(&url::Url) -> bool + Send + 'static {
    move |to| {
        let Some(server) = local::server(&app).and_then(|server| server.parse().ok()) else { return true };
        let handle = app.clone();
        match navigation::decide(&server, to, local::on_this_computer(&app)) {
            Move::Allow => true,
            Move::SignInHere => {
                tauri::async_runtime::spawn(async move {
                    if let Ok(link) = local::sign_in_again() {
                        go(&handle, &link);
                    }
                });
                false
            }
            Move::SignIn => {
                tauri::async_runtime::spawn(async move { signin::begin(&handle) });
                false
            }
            Move::PickFolder => {
                tauri::async_runtime::spawn(async move { folders::pick(&handle) });
                false
            }
            Move::Instead(page) => {
                tauri::async_runtime::spawn(async move { go(&handle, page.as_str()) });
                false
            }
            Move::Browser => {
                let address = to.to_string();
                tauri::async_runtime::spawn(async move {
                    let _ = handle.opener().open_url(address, None::<&str>);
                });
                false
            }
        }
    }
}

fn build(app: &AppHandle, target: WebviewUrl) -> tauri::Result<WebviewWindow> {
    let window = WebviewWindowBuilder::new(app, LABEL, target)
        .title("Latentic Agent Cloud")
        .inner_size(1200.0, 820.0)
        .min_inner_size(640.0, 480.0)
        .initialization_script("window.latenticDesktop = true;")
        .on_navigation(allowed(app.clone()))
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

/// The chat is where the app opens: the one place to write to an agent, choosing which agent and model.
pub fn chat_address(server: &str) -> String {
    format!("{}/chat", server.trim_end_matches('/'))
}

/// Shows the window: the server's pages when the app is connected to one, and the page that asks for its address when it is not.
pub fn show(app: &AppHandle) {
    if let Some(window) = main_window(app) {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
        return;
    }
    let target = match local::server(app).and_then(|server| chat_address(&server).parse().ok()) {
        Some(chat) => WebviewUrl::External(chat),
        None if local::on_this_computer(app) => WebviewUrl::App("index.html#starting".into()),
        None => WebviewUrl::App("index.html".into()),
    };
    if let Ok(window) = build(app, target) {
        let _ = window.set_focus();
    }
}

/// Runs a line of script in the page the window is showing, which is how a page is told what the app did for it.
pub fn tell_page(app: &AppHandle, script: &str) {
    if let Some(window) = main_window(app) {
        let _ = window.eval(script);
    }
}

/// Shows an address of the server in the window, opening it if it is not open.
pub fn go(app: &AppHandle, address: &str) {
    show(app);
    if let (Some(window), Ok(address)) = (main_window(app), address.parse()) {
        let _ = window.navigate(address);
    }
}

/// Finds or starts gusto on this computer and signs the window in to it, or says on the page why it could not.
pub fn start_here(app: &AppHandle) {
    let app = app.clone();
    std::thread::spawn(move || match local::start(&app) {
        Ok(link) => go(&app, &link),
        Err(problem) => {
            show(&app);
            tell_page(&app, &format!("window.cannotStart && window.cannotStart({})", serde_json::to_string(&problem).unwrap_or_default()));
        }
    });
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

#[cfg(test)]
mod tests {
    use super::chat_address;

    #[test]
    fn the_chat_is_under_the_server() {
        assert_eq!(chat_address("https://gusto.example.com"), "https://gusto.example.com/chat");
        assert_eq!(chat_address("http://127.0.0.1:8040/"), "http://127.0.0.1:8040/chat");
    }
}
