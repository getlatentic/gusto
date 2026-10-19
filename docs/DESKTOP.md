# The desktop side

The agents stay in the person's workspace in the organisation's cloud, and a turn keeps running when the laptop closes. The desktop side runs no agent. The app is a window onto the server's pages, and `gusto-helper` does the local work for the app, editors, the terminal and folders.

## The app

The app opens on the chat page and adds the helper, a menu bar or tray icon, notifications, `latentic://` links, start at login, and updates. The source is in `desktop/`.

A release carries the app for macOS (Apple silicon and Intel, a disk image), Windows on Intel (an installer) and Linux on Intel (an AppImage and a package). It is not built for Windows or Linux on Arm. It is not signed with a platform certificate yet (getlatentic/planning#601), so macOS and Windows ask you to allow it the first time.

## Working in a folder

The folder button on the chat page opens a window like a Finder open panel: projects down the side (a project is an agent's folder), the path along the top, and the folders and files in a list. Double-click opens a folder, **Choose** picks it, **New Folder** makes one (at the top it makes a project). Every turn of the chat starts in that folder. Nothing outside an agent's workspace can be named. This works in the browser and the app.

The app adds **Sync one from this computer**. It opens the system's folder chooser, and the helper keeps that folder the same as an agent's folder in the cloud, by the rule under [Files](#files). The server holds a key made of the folder's path and name, never the path. The helper refuses a whole home folder or disk.

## Updates

The app asks its server for `/desktop/latest.json` shortly after it starts and every six hours, and offers an update in its menu. It checks each update against a public key it holds. A server with no folder to hand out offers no updates. To hand out a release:

```bash
deploy/desktop/publish.sh v0.1.0-rc.2 /srv/gusto/desktop https://gusto.example.com
GUSTO_DESKTOP=/srv/gusto/desktop gusto serve
```

`publish.sh` needs the GitHub command line, signed in. It writes a `latest.json` that points at that server. Run it again for each release.

The release workflow signs updates with the repository secret `TAURI_SIGNING_PRIVATE_KEY`. The public half is in `desktop/src-tauri/tauri.conf.json`. If the private key is lost, no installed app can be updated again.

## Installing the helper

Each release has `gusto-helper` and `gusto` for macOS, Linux and Windows on Intel and Arm, as `gusto-helper_<release>_<system>_<processor>.tar.gz` and `gusto_<release>_<system>_<processor>.tar.gz` (`.zip` on Windows), with a `SHA256SUMS` signed with cosign. While the releases are private, the scripts need the GitHub command line, signed in.

```bash
gh release download v0.1.0-rc.2 -R getlatentic/gusto -p install.sh -O - | sh -s -- v0.1.0-rc.2
```

```powershell
gh release download v0.1.0-rc.2 -R getlatentic/gusto -p install.ps1 -O install.ps1; powershell -ExecutionPolicy ByPass -File install.ps1 -Tag v0.1.0-rc.2
```

The script checks the packages against the checksums, and the signature when `cosign` is installed (`GUSTO_REQUIRE_COSIGN=1` refuses to go on without it). It installs to `~/.local/bin`, or on Windows to `%LOCALAPPDATA%\gusto`, which it adds to your PATH. `GUSTO_INSTALL_DIR` changes the folder and `GUSTO_PROGRAMS=gusto-helper` installs the helper alone. `gusto version` and `gusto-helper version` print the release. The scripts are `deploy/install/install.sh` and `install.ps1`, and `deploy/helper/build.sh` builds the packages.

The programs are not signed with an Apple or Windows certificate. macOS refuses a package downloaded in a browser until you allow it in Privacy & Security, and Windows may show SmartScreen. The checksum and the cosign signature vouch for the package.

## The helper

`gusto-helper` has `login`, `whoami`, `logout`, `run`, `status`, `service`, `sync` and `acp` (`cmd/gusto-helper`, `internal/helper/cli`). `login` uses the device flow ([TOKENS.md](TOKENS.md)), and `logout` ends the token on the server. `gusto` also builds for Windows, for `login`, `whoami`, `submit`, `verify` and `show`.

`gusto-helper run` keeps one connection to the server's event stream, reconnects after a drop, and follows a sign-in or sign-out without a restart. A second `run` is refused while one answers. Its local API listens on `127.0.0.1` at a port the system picks. The address and a token are in `helper.json` in the helper's settings folder, readable by you alone. A request goes to that address (or `localhost` on its port) with `Authorization: Bearer <token>`. The routes are `GET /v1/status`, `/v1/turns` and `/v1/events`.

`gusto-helper service install -server <address>` starts the helper at login and now: a launch agent on macOS, a systemd user unit on Linux, a scheduled task at logon on Windows. None needs an administrator. `uninstall` removes it and `status` says whether it is there. On Linux, `loginctl enable-linger` keeps it running when you are logged out.

## Files

`gusto-helper sync -folder ~/work/project` keeps that folder the same as your agent's folder in the cloud, in both directions, as real files. `-dry-run` prints what it would do and does nothing, `-once` compares once, and `-force` goes on after a safety stop. A save, or any word of the agent's turns, starts a comparison, after the saves stop for 0.3 s. A timed comparison runs every five minutes as a backstop. The helper records what both sides held when they last agreed in `sync/` in its settings folder. The rule is in `internal/helper/mirror/plan.go`:

- A change on one side only goes to the other, including a new file or a deletion.
- A change beats a deletion. A file changed on one side and deleted on the other stays on both.
- When both sides changed a file, nothing is merged and nothing is lost. The local file stays. The cloud's version is saved beside it as `<name>.conflict-<time>.<ext>` (`report.conflict-20261008T143005Z.md`), and both go to the cloud.

The cloud refuses an upload over a file other than the one the helper saw (`X-If-Hash`), and the next run treats that as a conflict. The helper does not follow links, and refuses a path that leaves the folder or goes through a link. It leaves alone a file over 64 MiB.

It stops before acting, unless you pass `-force`, when:

- the folder is your whole home or the root of a disk;
- a first run covers more than 20,000 files;
- a run would delete 20 or more files and half of what is kept;
- the cloud listing was cut short.

It leaves out `.git/`, `node_modules/`, `.venv/`, `__pycache__/`, `*.pyc` and operating-system litter. A `.gustoignore` in the folder's root adds more, one pattern a line: a name (`*.log`), a folder (`build/`) or a path from the root (`docs/drafts`). A `!` line is not understood. An ignored file is never deleted.

Empty folders are not kept. A file the agent writes crosses when its turn ends or changes state. Two names that differ only in case are two files, which a case-insensitive disk (the default on macOS and Windows) cannot hold. Only the executable bit is kept. Sync has not been run on Windows or on a very large folder.

## Editors

`gusto-helper acp` is the helper as an agent for an editor that speaks the [Agent Client Protocol](https://agentclientprotocol.com). The editor starts it and they talk over standard input and output. Logs go to standard error.

| The editor sends | The helper does |
| --- | --- |
| `initialize` | Answers protocol version 1, with no `loadSession`, no images or audio, and no authentication to do |
| `session/new` | Opens a thread on one of your cloud agents: the one named by `-instance`, else your newest, else a new one of the kind `-agent` names |
| `session/prompt` | Says it in the thread and follows the turn. Each step is a `tool_call`, and the reply arrives as `agent_message_chunk`s |
| `session/cancel` | Stops the turn in the cloud ([STATES.md](STATES.md#stopping-a-task)), and the prompt ends `cancelled` |

A failed turn is reported in words and ends `end_turn`. Closing the editor ends the helper, not the turn. Signed out, `session/new` returns the authentication error: run `gusto-helper login`, with no editor restart. The agent works in its cloud folder, not the editor's open folder, and says so first; `cwd` is ignored (getlatentic/planning#502). `session/load`, the editor's file and terminal methods, and MCP servers an editor passes are not used.

Setup, from the editors' own documentation (neither tried):

```jsonc
// Zed, settings.json
{ "agent_servers": { "gusto": { "type": "custom", "command": "gusto-helper", "args": ["acp", "-server", "https://gusto.example.com"], "env": {} } } }

// JetBrains, ~/.jetbrains/acp.json
{ "agent_servers": { "gusto": { "command": "/path/to/gusto-helper", "args": ["acp", "-server", "https://gusto.example.com"] } } }
```

Zed's `dev: open acp logs` shows the conversation with the helper.

## Attention

`gusto-helper run` shows an icon in the menu bar or tray (`-tray=false` turns it off). Its menu shows how the helper stands and up to eight turns, running first. A click on a turn opens its conversation in the browser. A notification names a turn that finished, failed or was stopped, and is not clickable. A turn waiting for approval is not announced. `gusto-helper status` reports `icon: shown`, `icon: none: ...` or `icon: off`. With no icon, sync, the editor bridge and notifications keep working.

| Where | The icon |
| --- | --- |
| macOS | In the menu bar. Tried on macOS 27 only. A helper built without a C compiler has none |
| Windows | In the notification area. Not tried by a person |
| Linux with KDE Plasma or another status-notifier desktop | Shown when a program on the session bus owns `org.kde.StatusNotifierWatcher` |
| Linux with GNOME | Only with the AppIndicator extension ("AppIndicator and KStatusNotifierItem Support") |
| Linux with no desktop session or no tray | None. The helper says so and runs |

Linux notifications use `notify-send`, and Windows notifications use a toast.
