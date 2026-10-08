//! Updates: the app asks the server it is connected to whether there is a newer one, and installs it when the person says. The
//! server hands the files out and cannot vouch for them: each is signed with a key whose public half is in this app, and an update
//! that does not verify is refused.

use crate::config;
use std::{sync::Mutex, time::Duration};
use tauri::{AppHandle, Manager};
use tauri_plugin_updater::{Update, UpdaterExt};

/// The newer version that was found, which waits to be installed.
#[derive(Default)]
pub struct Available(Mutex<Option<Update>>);

/// The version found, if one was.
pub fn found(app: &AppHandle) -> Option<String> {
    app.state::<Available>().0.lock().unwrap().as_ref().map(|update| update.version.clone())
}

/// Asks, a little after the app starts and then every six hours, while it is connected to a server.
pub fn watch(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        tokio_sleep(30).await;
        loop {
            check(&app).await;
            tokio_sleep(6 * 60 * 60).await;
        }
    });
}

async fn tokio_sleep(seconds: u64) {
    tauri::async_runtime::spawn_blocking(move || std::thread::sleep(Duration::from_secs(seconds))).await.ok();
}

async fn check(app: &AppHandle) {
    let Some(server) = config::load(app).server else { return };
    let Ok(endpoint) = format!("{server}/desktop/latest.json").parse() else { return };
    let Ok(builder) = app.updater_builder().endpoints(vec![endpoint]) else { return };
    let Ok(updater) = builder.build() else { return };
    // A server with nothing to hand out, or one that cannot be reached, is not a problem the person has: it is asked again later.
    if let Ok(newer) = updater.check().await {
        *app.state::<Available>().0.lock().unwrap() = newer;
    }
}

/// Downloads the update that was found, verifies it against the key in this app, installs it, and starts the new app.
pub fn install(app: &AppHandle) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let update = app.state::<Available>().0.lock().unwrap().take();
        let Some(update) = update else { return };
        if update.download_and_install(|_, _| {}, || {}).await.is_ok() {
            crate::helper::stop(&app);
            app.restart();
        } else {
            // It stays on offer, so that the person can try again.
            *app.state::<Available>().0.lock().unwrap() = Some(update);
        }
    });
}
