//! gusto for a person's own computer: a window on gusto, on this computer or on their organization's server, an icon that
//! says what their agents are doing, and a notification when one finishes. On this computer the app uses gusto as the install script
//! left it, or starts the one it carries; for an organization the work is done by gusto-helper, which the app starts and keeps going.

mod config;
mod folders;
mod helper;
mod links;
mod local;
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
    local::stop(&app);
    helper::start(&app, &server);
    window::go(&app, &window::chat_address(&server));
    Ok(())
}

/// Uses gusto on this computer: forgets the organization's server, ends the helper that followed it, and starts or finds gusto here.
#[tauri::command]
fn use_this_computer(app: AppHandle) -> Result<(), String> {
    config::save(&app, &config::Config { server: None })?;
    helper::stop(&app);
    window::start_here(&app);
    Ok(())
}

/// A menu to go back and forward in the window, which has no address bar to do it with: a link that leads somewhere unwanted is not a
/// place the person is stuck in.
fn history_menu(app: &mut tauri::App) -> tauri::Result<()> {
    use tauri::menu::{Menu, MenuItem, Submenu};
    let handle = app.handle().clone();
    let menu = Menu::default(&handle)?;
    let history = Submenu::with_items(
        &handle,
        "History",
        true,
        &[
            &MenuItem::with_id(&handle, "back", "Back", true, Some("CmdOrCtrl+["))?,
            &MenuItem::with_id(&handle, "forward", "Forward", true, Some("CmdOrCtrl+]"))?,
            &MenuItem::with_id(&handle, "reload", "Reload", true, Some("CmdOrCtrl+R"))?,
        ],
    )?;
    menu.append(&history)?;
    app.set_menu(menu)?;
    app.on_menu_event(|app, event| match event.id().as_ref() {
        "back" => window::tell_page(app, "history.back()"),
        "forward" => window::tell_page(app, "history.forward()"),
        "reload" => window::tell_page(app, "location.reload()"),
        _ => {}
    });
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
        .manage(local::Local::default())
        .manage(tray::Pages::default())
        .manage(updates::Available::default())
        .manage(signin::Pending::default())
        .invoke_handler(tauri::generate_handler![connect, use_this_computer])
        .setup(|app| {
            let handle = app.handle().clone();
            history_menu(app)?;
            tray::build(&handle)?;
            updates::watch(handle.clone());
            match config::load(&handle).server {
                Some(server) => helper::start(&handle, &server),
                None => window::start_here(&handle),
            }
            window::show(&handle);
            // For finding out why notifications do not appear on a computer: the system asks permission the first time, and a click on this one shows the window.
            if std::env::args().any(|argument| argument == "--test-notification") {
                let server = local::server(&handle).unwrap_or_default();
                notice::send(&handle, "Your agent finished", "This is a test of the notification", &server);
            }
            let linked = handle.clone();
            app.deep_link().on_open_url(move |event| {
                let server = local::server(&linked).unwrap_or_default();
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
        RunEvent::Exit => {
            helper::stop(app);
            local::stop(app);
        }
        #[cfg(target_os = "macos")]
        RunEvent::Reopen { .. } => window::show(app),
        _ => {}
    });
}
