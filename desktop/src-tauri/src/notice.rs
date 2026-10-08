//! A notification that ends in the window: a click on it shows the turn it is about. Each system is asked in its own way, since no one
//! crate does it on all three.

use tauri::AppHandle;

/// Says a turn ended, and shows `page` in the window when the notification is clicked. It does not wait for the click.
pub fn send(app: &AppHandle, title: &str, body: &str, page: &str) {
    let app = app.clone();
    let (title, body, page) = (title.to_string(), body.to_string(), page.to_string());
    std::thread::spawn(move || deliver(&app, &title, &body, &page));
}

#[cfg(target_os = "macos")]
fn deliver(app: &AppHandle, title: &str, body: &str, page: &str) {
    use mac_notification_sys::{Notification, NotificationResponse};
    // The notification belongs to the app, and so shows its icon and name, once the app is a bundle with this identifier.
    let _ = mac_notification_sys::set_application(&app.config().identifier);
    let sent = Notification::new().title(title).message(body).wait_for_click(true).send();
    if let Ok(NotificationResponse::Click) = sent {
        crate::window::go(app, page);
    }
}

#[cfg(target_os = "windows")]
fn deliver(app: &AppHandle, title: &str, body: &str, page: &str) {
    use tauri_winrt_notification::Toast;
    let handle = app.clone();
    let page = page.to_string();
    let _ = Toast::new(&app.config().identifier)
        .title(title)
        .text1(body)
        .on_activated(move |_| {
            crate::window::go(&handle, &page);
            Ok(())
        })
        .show();
}

#[cfg(all(unix, not(target_os = "macos")))]
fn deliver(app: &AppHandle, title: &str, body: &str, page: &str) {
    let shown = notify_rust::Notification::new().appname("Latentic Agent Cloud").summary(title).body(body).action("default", "Open").show();
    if let Ok(handle) = shown {
        handle.wait_for_action(|action| {
            if action == "default" {
                crate::window::go(app, page);
            }
        });
    }
}
