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

## Settings

The server's address is in `config.json` in the app's settings folder (`~/Library/Application Support/com.latentic.agentcloud` on
macOS). The helper's own log is `helper.log` in the app's log folder.
