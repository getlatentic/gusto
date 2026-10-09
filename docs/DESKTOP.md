# The desktop side

How a person reaches their organisation's agents from their own computer. This is
the design for getlatentic/planning#530. Nothing here is built yet.

## The shape

The agents stay in the person's cloud computer, and a turn keeps running when the
laptop closes. The desktop side is a client. It is not a harness, and it runs no
agent.

```
 editor ──── haas acp ───┐
 terminal ── haas ───────┤
 tray ───────────────────┼── haas helper (Go, on the person's computer) ──── the organisation's server
 folder ── sync ─────────┘        │
                                  └── token in the operating system's keychain
```

One small Go program, the helper, does the local work, so that an editor, the
terminal, a folder and a notification all reach the cloud the same way. A shell
(a tray, maybe a window) is a thin layer over the helper's local API. Which shell,
or none, is the spike in getlatentic/planning#587.

## What the server does not have yet

The server is built around the web pages. Of its routes, three are an API
(`POST /api/jobs`, `GET /api/changes`, `GET /api/spend`), and their tokens are the
organisation's, made by an admin, with the scopes `submit`, `spend`, `audit` and
`metrics`. A person's instances, threads, turns and workspace files can be read
only as HTML, behind a session cookie.

So the helper cannot be built on what is there. It needs two things on the server
first, which getlatentic/planning#588 files:

1. **A token for a person.** The helper signs in as the person, not as the
   organisation. The flow is the device flow: the helper asks the server for a
   code and opens the browser, the person signs in as usual (SSO or password) and
   approves the code, and the helper receives a token tied to that person. The
   person can revoke it from the web, and every use of it is in the record of what
   changed on the platform.
2. **A JSON API for a person's own work.** Instances, threads, turns with their
   steps, approvals, the workspace's file list and its changes, and one event
   stream that says when a turn starts, finishes, fails or waits for an approval.
   Every route answers about the signed-in person's own objects, and a
   somebody-else's id answers as missing, as the pages do.

The same API is what a native UI would need. A native UI is not planned, and is not
a reason to build this API.

## The app

The app is a window onto the server's pages, with the helper beside it, an icon in the menu bar or tray, notifications, `latentic://`
links, start at login, and updates. It opens on the chat page: a box to write in, with the agent and model the message goes to
chosen under it. The source is in `desktop/`.

A release carries the app for macOS on Apple silicon and Intel (a disk image), Windows on Intel (an installer) and Linux on Intel (an
AppImage and a package). It is not built for Windows or Linux on Arm. None of it is signed with a platform certificate yet
(getlatentic/planning#601), so macOS and Windows ask the person to allow it the first time.

### Working in a folder

The agent works on a computer in the cloud, so the folder a chat works in is a folder of that computer. The folder button on the chat page
opens a window laid out like a Finder's open panel: the projects down the side (a project is an agent's folder), the path along the top, and
the place's folders and files in a list with their dates and sizes. A folder is opened with a double click, chosen with **Choose**, and made
with **New Folder** (at the top it makes a new project). The side list of conversations is grouped by project in the same way. The chat then goes to that agent,
and every turn of it starts in that folder. Nothing outside an agent's workspace can be named, by a path, a link or somebody else's agent.
This is in the browser as well as the app.

In the app the explorer has one more choice, **Sync one from this computer**: it opens the system's folder chooser, and the helper keeps
the folder chosen the same as an agent's folder in the cloud, in both directions, by the rule in [Files](#files). The server holds only a
key made of the folder's path and the folder's name, never the path. Another chat in the same folder goes to the same agent. The helper
will not keep a whole home folder or disk.

### Updates

The app asks the server it is connected to for `/desktop/latest.json`, a little after it starts and every six hours, and offers the
update in its menu. It checks the update against a key it holds the public half of, so a server hands files out and cannot vouch for
them. A server with no folder hands out nothing, and its people are not offered updates.

To hand out a release, give the server a folder and put the release in it:

```bash
deploy/desktop/publish.sh v0.0.1-alpha.6 /srv/haas/desktop https://haas.example.com
HAAS_DESKTOP=/srv/haas/desktop haas serve
```

`publish.sh` needs the GitHub command line signed in, because the releases are private. It writes the `latest.json` for that one
server, whose addresses point at the server itself. Run it again for the next release.

The release workflow signs each update with the key in the repository secret `TAURI_SIGNING_PRIVATE_KEY`. The key's public half is in
`desktop/src-tauri/tauri.conf.json`; losing the private key means no installed app can be updated again.

## Installing the helper

Each release has the helper for macOS, Linux and Windows, on Intel and Arm, as
`haas-helper_<release>_<system>_<processor>.tar.gz` (a `.zip` on Windows), with a `SHA256SUMS`
that the release workflow signs with its own identity (cosign, keyless). The releases are
private, so the install scripts use the GitHub command line, signed in to an account that can
read them:

```bash
gh release download v0.0.1-alpha.5 -R getlatentic/haas -p install.sh -O - | sh -s -- v0.0.1-alpha.5
```

```powershell
gh release download v0.0.1-alpha.5 -R getlatentic/haas -p install.ps1 -O install.ps1; ./install.ps1 -Tag v0.0.1-alpha.5
```

The script downloads the package for this system, checks it against the checksums, checks the
checksums' signature when `cosign` is installed (`HAAS_REQUIRE_COSIGN=1` refuses to go on
without it), and puts `haas-helper` in `~/.local/bin` (`HAAS_HELPER_DIR` changes that) or, on
Windows, in `%LOCALAPPDATA%\haas-helper`, which it adds to the user's PATH: the only setting it
changes. `haas-helper version` says which release is installed. The packages are built by
`deploy/helper/build.sh`. Linux and Windows programs are built with no C compiler. macOS programs
are built on macOS, because the menu bar icon is drawn through the system's libraries. No package
holds a path of the build machine.

**Not signed for the operating systems.** The programs are not signed with an Apple developer
certificate or a Windows code-signing certificate, which this project does not have. A package
downloaded with `gh` or `curl` is not quarantined, so macOS runs it; one downloaded in a
browser is, and macOS refuses it until it is allowed in Privacy & Security. Windows may show
SmartScreen. The checksum and the cosign signature are what vouch for it until then.

## The helper

It is its own program, `haas-helper` (`cmd/haas-helper`), with its commands in
`internal/helper/cli`, so that a person's computer does not carry the server's
program. `haas` itself builds for Windows too, for the verbs a person uses there
(`login`, `whoami`, `submit`, `verify`, `show`); the server's own verbs need Docker
and a Linux box, and are not what Windows is for. Both programs build without a C
compiler for macOS, Windows and Linux, on Intel and Arm, and a workflow builds and
vets all of it for all six on each Go change, and runs the helper's tests and the real
Credential Manager on a Windows runner.

Built so far: `login` by the device flow, `whoami`, `logout` (which ends the token on
the server), `run`, `status` and `service`.

**`haas-helper run`** is the helper. It keeps one connection to the server's event
stream, gets it back after it drops (asking the server to catch up from a minute
before the last event it heard), and tells every program on the computer what it
hears. A person who signs in or out while it runs needs no restart: it looks again.

**The local API** is on `127.0.0.1` at a port the system picks. How to reach it is in
`helper.json` in the helper's settings folder, written for its owner alone: the address
and a token made when the helper started. Every request has to come to that address
(or `localhost` on its port), which keeps a web page out that reaches this computer by
another name, and has to carry the token as `Authorization: Bearer`.

| Route | Answers |
| --- | --- |
| `GET /v1/status` | the server it follows, whether the person is signed in, who, and whether it is connected |
| `GET /v1/turns` | the latest word on each of the person's jobs, newest first |
| `GET /v1/events` | the same events the server sends (`turn.queued`, `turn.started`, `turn.finished`, `turn.failed`, `turn.interrupted`): each job's latest, then `ready`, then each change |

A second `run` refuses while one answers. No route has cross-origin headers: a shell
that is a web view on another origin needs the helper told to allow it. The tray
(see Attention) is part of the helper, so it needs no such route.

**`haas-helper service install -server <address>`** makes the helper start at login
and starts it now, with what each system has for a program a person runs: a launch
agent on macOS, a user unit of systemd on Linux, and a scheduled task at logon on
Windows. None needs an administrator. `uninstall` stops it and removes the start,
and `status` says whether it is there. A second install replaces the first. On Linux,
`loginctl enable-linger` keeps it running when the person is not logged in. Tried on
a real macOS launch agent: it started, restarted after it was killed, and left nothing
after `uninstall`. The Windows scheduled task is installed, queried and removed on a
Windows runner by the builds workflow.



## Files

`haas-helper sync -folder ~/work/project` keeps that folder the same as the person's agent's
folder in the cloud, in both directions, as real files on both sides. There is no virtual
file system and no kernel extension: those are three hard integrations (macOS File
Provider, Windows Cloud Files, Linux FUSE), and are added only when a measured workspace is
too large to keep this way. `-dry-run` says what it would do and does nothing, `-once`
compares once, and without either it keeps the folders. A save in the folder starts a
comparison, and so does any word of the agent's turns: the end of a turn is when its files have
just changed. A burst of saves, such as a build or a `git checkout`, is one comparison, which
starts when the saves have stopped for 0.3 s (3 s at most). A comparison that did something is
followed by another in three seconds. When nothing has changed, the timed comparison is the
backstop for a watcher that drops an event, every five minutes. A build with no folder watcher
compares every three seconds while there are changes and every thirty when there are none.
The watcher is FSEvents on macOS (one stream for the tree), and inotify and
ReadDirectoryChangesW elsewhere, with one watch for each folder. When Linux's
`fs.inotify.max_user_watches` is reached, the helper says so and the timed comparison keeps the
rest. Only a workspace is shared, never the rest of the cloud
computer.

**The rule** is in `internal/helper/mirror/plan.go`, with its tests, and is the whole of what
decides what happens to a file. The helper keeps a record of what both sides held when they
last agreed, so it can tell a change from a deletion, and a change on one side from a change
on both.

- A change on one side only goes to the other: a file that is new, changed or deleted on one
  side while the other did not touch it is made so on the other.
- **A change beats a deletion.** If one side deleted a file and the other changed it, the
  changed file stays, on both sides. Nothing somebody edited is removed because somebody else
  removed the file.
- **When both sides changed a file**, or both made a new one of the same name with different
  content, neither version is lost and neither is merged. The local file stays where it is,
  as it is, with its name. The cloud's version is saved beside it as
  `<name>.conflict-<time>.<ext>` (`report.conflict-20261008T143005Z.md`), and both then go
  to the cloud, so the agent has them too. The local side wins the name because a person
  saving a file by hand while an agent edits it is the person's act, and what the agent wrote
  is one rename away. The helper says so when it happens.
- A property test over thousands of random pairs of folders shows that after a plan the two
  sides agree, a second plan has nothing to do, and no change anybody made is gone from both.

**What it will not do.**

- It does not overwrite a file that changed while it was working: the local file is looked at
  again after a download and before it is replaced, and the cloud refuses an upload over a file
  that is not the one the helper saw (`X-If-Hash`). That is looked at again next time, which is
  when it becomes a conflict, not a loss.
- It does not follow links, and a path from the cloud that would leave the folder or go
  through a link is refused, on both sides. A file over 64 MiB is left where it is on either
  side, and is never taken for a deleted one.
- It stops before doing anything when the run looks like a mistake: a folder that is your
  whole home or the root of a disk; more than 20,000 files on a first run; a run that would
  delete 20 or more files and half of what is kept (a disk that did not mount, a folder
  moved); and a listing in the cloud that was cut short, in which the files not listed would
  look deleted. `-force` goes on anyway.
- It leaves out `.git/` (the agent's copy and the person's are two clones), `node_modules/`,
  `.venv/`, `__pycache__/`, `*.pyc` and operating-system litter, and whatever a `.haasignore`
  in the folder's root adds, one pattern to a line: a name (`*.log`), a folder (`build/`), or
  a path from the root (`docs/drafts`). A `!` line is not understood and is said so. An
  ignored file is never deleted on either side.

**What it does not do yet.** Empty folders are not kept. A file the agent writes crosses when
its turn ends or changes state, and not while the turn is running.
Names are compared exactly, so two names that differ only in case are two files, which a
case-insensitive disk (the default on macOS and Windows) cannot hold. An executable file is
kept executable, but no other mode, owner or time is. The record is in the helper's settings
folder, in `sync/`, one file for each pair of server, agent and folder.

**What was checked.** The rule's tests; the engine against an in-memory cloud, with a person
saving during a download, the agent writing during an upload, a conflict that cannot be sent,
links, large files, paths that leave the folder, the guards, ignored files, and a restart; and
the whole of it against the real server, where the agent writes into its folder by the file
system as it does. The server's own checks are in [TOKENS.md](TOKENS.md). Nothing has been run
on Windows, and not against a very large folder.

## Editors

`haas-helper acp` is the helper as an agent for an editor that speaks the
[Agent Client Protocol](https://agentclientprotocol.com): the editor starts it, and they talk
on standard input and output, one JSON-RPC message to a line. Nothing else is written to
standard output; what the helper has to say goes to standard error, where an editor's
log shows it.

| The editor does | The helper does |
| --- | --- |
| `initialize` | answers protocol version 1, with no `loadSession`, no images or audio, and no authentication to do |
| `session/new` | opens a thread on one of the person's agents in the cloud: the one named by `-instance`, else their newest, else a new one of the kind `-agent` names (the first this server can run). All an editor's sessions are on that one agent. The thread's id is the session's |
| `session/prompt` | says it in the thread with a request id made from the session, the turn's place and the words, so a send that is tried again is one turn, and follows the turn: each step is a `tool_call` (kind `read`, `edit`, `execute`, `search`, `fetch`, `think` or `other` by the verb), and what the agent says arrives as `agent_message_chunk`s of only the new words |
| `session/cancel` | stops the turn in the cloud ([STATES.md](STATES.md#stopping-a-job)), and the prompt ends `cancelled` when the turn's own end arrives. A cancel that comes before the turn was sent stops it as it is sent |

A turn that fails is said so in words after what the agent said, and ends `end_turn`, since the
protocol's reasons have none for a failure. A reply the server cut short, as it does past
40 KB, is not told again from its new start: an editor cannot take words back.

Closing the editor ends the helper and not the turn: the agent is in the cloud, and the turn
finishes there. Signed out, `session/new` answers with the protocol's authentication error
and says to run `haas-helper login`; signing in later needs no restart of the editor.

**What the editor does not get yet.** The agent works in its folder in the cloud, not in the
folder the editor has open: the first thing it says in a session is that. `cwd` is read and
ignored until the two folders are kept in step (getlatentic/planning#502). `session/load`
is not offered, though threads persist and a reopened session could be replayed. The
editor's file and terminal methods are not used, since the agent is not on the person's
machine. MCP servers an editor passes are ignored.

**Setting it up.** From the two editors' own documentation (neither tried here):

```jsonc
// Zed, settings.json
{ "agent_servers": { "haas": { "type": "custom", "command": "haas-helper", "args": ["acp", "-server", "https://haas.example.com"], "env": {} } } }

// JetBrains, ~/.jetbrains/acp.json
{ "agent_servers": { "Latentic Agent Cloud": { "command": "/path/to/haas-helper", "args": ["acp", "-server", "https://haas.example.com"] } } }
```

Zed's `dev: open acp logs` shows the conversation with the helper.

**What was checked.** The bridge is driven in tests by a scripted editor, and against the
real server, for an answer and for a cancel that really stops the turn. The official
TypeScript client library (`@agentclientprotocol/sdk` 1.7.0, which checks messages against the
protocol's schema) was also run against `haas-helper acp` and a stub of the server:
`login` by the device flow, `initialize`, `session/new`, a prompt with steps and a reply, and a
cancel that ends `cancelled` and reaches the server as a stop. No editor has been used.

## Attention

`haas-helper run` shows an icon in the menu bar or tray (`-tray=false` turns it off). Its menu says
how the helper stands and lists up to eight turns, running ones first. A click on a turn opens its
conversation in the browser. A notification names a turn that finished, failed or was stopped, and
only a turn the helper saw going: the turns the server repeats on a reconnect are silent. A turn
that waits for an approval is not announced, because the server sends no event for it yet.
A computer with nowhere to show the icon runs the helper with no icon, and `haas-helper status` says so and why (`icon: shown`, or
`icon: none: ...`, or `icon: off` when it was started with `-tray=false`). Everything else, which is the sync, the editor bridge and the
notifications where the system has them, goes on. A macOS helper built without a C compiler has no icon either.
There is no window in the first version. A person who wants
the conversation uses the web, an editor or the terminal.

### Where the icon shows

| Where | The icon |
| --- | --- |
| macOS | Always, in the menu bar. Tried on macOS 27 only |
| Windows | In the notification area. Not tried: only the build and the program's output are checked, on a Windows runner |
| Linux with KDE Plasma, or another desktop that shows status notifier icons | Shown, when a program on the session bus owns `org.kde.StatusNotifierWatcher`. Checked on a real session bus in a container, and not on a real desktop |
| Linux with GNOME | Only with the AppIndicator extension ("AppIndicator and KStatusNotifierItem Support"). Without it the helper says so and runs with no icon |
| Linux with no desktop session, such as a server | None. The helper runs without one |
| Another Linux desktop that shows no tray icons | None, and the helper says so |

Notifications on Linux go through `notify-send` and on Windows through a toast, so they depend on the desktop too; they are not
clickable (see above). The tray is never the only way in: the web, an editor and the terminal reach everything it shows.

On Windows the helper is built as a program with no window of its own, so that starting it at login does not open a black one. When
it is started from a terminal it attaches to that terminal, so `haas-helper login` and `status` print there as they do elsewhere.
Linux desktops and Windows were not tried by a person; what is checked is stated in the table.

The tray is part of the helper process. There is no shell around it. The spike
(getlatentic/planning#587) built the same tray, with one notification, three ways on
macOS 27 (Apple silicon):

| | Tauri 2 | Wails v3 (beta.28) | Helper alone (`fyne.io/systray`) |
| --- | --- | --- | --- |
| Toolchains | Go and Rust | Go | Go |
| Cold build | 3 min 17 s | 55 s | 4.5 s |
| Binary | 3.5 MB | 15.4 MB | 4.2 MB |
| Idle memory | 87.5 MB | 82.0 MB | 75.4 MB |
| Extra process for the helper | yes | no | no |
| Notification needs an app bundle | yes | yes (refuses to start without one) | no |
| Click on a notification opens a page | yes, in a bundle | yes, in a bundle | no |

The helper alone wins on build, size, memory and parts. Its cost is that a
notification cannot be clicked: on macOS it comes from `osascript` and belongs to
Script Editor. The tray menu holds the same turns, so a click there opens the page.
A shell returns when the helper needs a window. Tauri is then the choice, because
Wails v3 is still a beta. Measured on macOS only: the Windows and Linux trays are
not measured. The `latentic://` link and the notification permission prompt were
not shown working in either bundle, so neither is a reason to choose.

## What is not decided

- A native UI, which waits on people asking for more than this and needs the API
  above.

## Not taken

- **Coder Desktop** has one codebase for each operating system and is AGPL.
- **Tabby** is terminal-first, and the terminal is one surface here.
- **Taking over a harness's terminal** waits on the `terminal-pane` branch.
