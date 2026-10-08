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
that is a web view on another origin needs the helper told to allow it, which is
decided with the shell (getlatentic/planning#587).

**`haas-helper service install -server <address>`** makes the helper start at login
and starts it now, with what each system has for a program a person runs: a launch
agent on macOS, a user unit of systemd on Linux, and a scheduled task at logon on
Windows. None needs an administrator. `uninstall` stops it and removes the start,
and `status` says whether it is there. A second install replaces the first. On Linux,
`loginctl enable-linger` keeps it running when the person is not logged in. Tried on
a real macOS launch agent: it started, restarted after it was killed, and left nothing
after `uninstall`. The Windows scheduled task is installed, queried and removed on a
Windows runner by the builds workflow.

Next: folder sync.

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
