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

## The helper

It is its own program, `haas-helper` (`cmd/haas-helper`), with its commands in
`internal/helper/cli`, so that a person's computer does not carry the server's
program. `haas` itself builds for Windows too, for the verbs a person uses there
(`login`, `whoami`, `submit`, `verify`, `show`); the server's own verbs need Docker
and a Linux box, and are not what Windows is for. Both programs build without a C
compiler for macOS, Windows and Linux, on Intel and Arm, and a workflow builds and
vets all of it for all six on each Go change, and runs the helper's tests and the real
Credential Manager on a Windows runner.

Built so far: `login` by the device flow, `whoami`, and `logout`, which ends the token
on the server. Next: the local API, `haas acp`, folder sync.

- **Sign-in.** The device flow above. The token goes in the operating system's
  keychain (macOS Keychain, Windows Credential Manager, the Secret Service on
  Linux), never in a file.
- **A local API on a random loopback port,** protected by a token the helper makes
  at start. A web page the person visits can try to call loopback, so the port and
  the token are what stop it. A shell is told both when it starts the helper.
- **Runs at login** as a user service on each operating system, and stops cleanly.
- **One Go codebase** for macOS, Windows and Linux, with no web view.

## Files

A folder the person picks is kept in step with a workspace in their cloud
computer, in both directions, as real files on the local disk. There is no virtual
file system and no kernel extension: those are three hard integrations (macOS File
Provider, Windows Cloud Files, Linux FUSE), and are added only when a measured
workspace is too large to sync.

- Only a workspace is shared. The rest of the cloud computer is not.
- An agent's change arrives locally as it is made, and a save arrives in the
  workspace before the next turn reads it.
- Ignore rules keep `node_modules` and build output off the wire.
- Local edits work offline and catch up on reconnect.

**The conflict rule.** The workspace and the local folder can both change the same
file. Neither change is lost. The helper keeps the local file as it is, writes the
workspace's version beside it as `<name>.conflict-<time>`, and says so in the tray
and in the next turn's context, so the agent knows. It never merges text, and never
picks a winner. The rule is written first, with tests, before any sync code.

## Editors

`haas acp` runs on the person's computer and speaks the Agent Client Protocol on
standard input and output, so Zed, JetBrains, the VS Code extensions and Neovim and
Emacs plugins start it as an agent. A session is a thread on one of the person's
instances. Closing the editor does not stop a turn. The server already speaks ACP
to its harnesses (`internal/acp`), so the helper maps the editor's session methods
onto instances and threads.

## Attention

A tray icon, or the menu bar, lists running, waiting and finished turns, and a
notification names a turn that finished or needs an approval. A click opens the
page in the browser. There is no window in the first version. A person who wants
the conversation uses the web, an editor or the terminal.

## What is not decided

- Whether the tray needs a shell. The spike builds the same thing in Tauri 2,
  Wails v3 and the helper alone, and measures it (getlatentic/planning#587).
  Wails v3 is a beta, and its signing and update support is checked, not assumed.
- A native UI, which waits on people asking for more than this and needs the API
  above.

## Not taken

- **Coder Desktop** has one codebase for each operating system and is AGPL.
- **Tabby** is terminal-first, and the terminal is one surface here.
- **Taking over a harness's terminal** waits on the `terminal-pane` branch.
