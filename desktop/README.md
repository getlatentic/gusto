# Latentic Agent Cloud for a computer

A window on your organisation's server, an icon that says what your agents are doing, and a notification when one finishes.
It is a [Tauri](https://tauri.app) app: a small Rust shell around the system's own web view, so the pages in the window are the server's
own pages and not a second interface. The work is done by `haas-helper` (sign-in, the synced folder, the editor bridge, following
your turns), which the app carries, starts with its own icon turned off, and keeps going.

| Part | What it does |
| --- | --- |
| `dist/index.html` | the page that asks for the server's address, shown until there is one |
| `src-tauri/src/config.rs` | keeps the address, and checks that a server answers there |
| `src-tauri/src/helper.rs` | starts the helper, starts it again when it ends, and asks it what it knows |
| `src-tauri/src/tray.rs` | the icon, its menu of turns, and the notification when a turn ends |
| `src-tauri/src/window.rs` | the window: closing it leaves the app and its icon running |
| `prepare.sh` | builds the helper the app carries, named for its platform |

## Links, notifications and login

- **A link** `latentic://open?path=/instances/<id>/threads/<id>` shows that page of the server the app is connected to. It is only ever
  a page of that server: a path that names another host, another scheme or no path is ignored, so a link in a message cannot take the
  window anywhere. (`src-tauri/src/links.rs`, with its tests.)
- **A notification** says a turn that was going has ended, and a click on it shows the turn in the window. macOS asks the first time
  whether the app may send notifications, and it must be allowed in System Settings for them to appear. `Latentic Agent Cloud
  --test-notification` sends one, to find out why they do not.
- **Start at login** is in the icon's menu, off until it is turned on.

## Build

You need Go, Rust (stable) and the Tauri command line, `cargo install tauri-cli --version "^2" --locked`. On Linux the web view's
libraries too (`libwebkit2gtk-4.1-dev`, `libayatana-appindicator3-dev`, `librsvg2-dev`).

```sh
./prepare.sh                       # the helper for this machine
cd src-tauri
cargo test
cargo tauri build --bundles app    # macOS: target/release/bundle/macos/Latentic Agent Cloud.app
```

The app is not signed yet (getlatentic/planning#601), so macOS asks you to allow it the first time.

## Updates

The app asks the server it is connected to for `/desktop/latest.json` 30 seconds after it starts, then every few hours. When the
file names a newer version, the tray shows "Update to X and restart". The app checks the minisign signature in the file against the
public key in `tauri.conf.json`, so the server does not need to be trusted.

To publish an update:

1. Build with the private key in the environment: `TAURI_SIGNING_PRIVATE_KEY=$(cat desktop-updater.key) cargo tauri build`.
2. Put `latest.json` and the update archives in one folder. The format is the Tauri updater format.
3. Start the server with `HAAS_DESKTOP=<folder>`. The server serves the folder under `/desktop/`.

A release does steps 1 and 2 for every system, and `deploy/desktop/publish.sh` does step 2 for one server ([DESKTOP.md](../docs/DESKTOP.md#the-app)).

## Settings

The server's address is in `config.json` in the app's settings folder (`~/Library/Application Support/com.latentic.agentcloud` on
macOS). The helper's own log is `helper.log` in the app's log folder.
