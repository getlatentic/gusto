# What each release changes for an operator

A release's notes list the images and the merged pull requests. This file says what
an operator has to know before they upgrade: settings added, defaults changed, and
things the server now refuses.

A tag needs a section here named for it. The release workflow stops without one and
copies the section into the release's notes. Changes made after a release go under
**Next release**, and that heading is renamed to the tag when the tag is made.
[UPGRADE.md](UPGRADE.md) says what happens to the data, and
[PILOT.md](PILOT.md) says how to install and check a release.

## Next release

**Changed**

- **opencode runs any model.** Its configuration names the job's own model, from `HAAS_MODEL_PROVIDER` and `HAAS_MODEL_ID`, in place of a fixed list, so a model an organisation adds works with it. Each engine's own configuration moved from `image/harness/` to `internal/harness/config/`; an image built from this repository copies it from there.
- **One install script for both programs.** `install.sh` and `install.ps1` install `haas` and `haas-helper` (`HAAS_PROGRAMS` names fewer) and are now in `deploy/install/`. On macOS and Linux they go to `~/.local/bin`, and `HAAS_INSTALL_DIR` names another folder (`HAAS_HELPER_DIR` is still read). On Windows they go to `%LOCALAPPDATA%\haas`, not `%LOCALAPPDATA%\haas-helper`: a person who installed the helper before has it in the old folder and on their PATH until they remove it. The scripts download with the GitHub command line when it is signed in, and otherwise from the release's public address.
- **Every model is offered to every engine** where a credential names no harnesses, and a new chat starts on the person's last choice, else the organisation's default (`/admin`, New chats), else `HAAS_CHAT_DEFAULT` (`engine=model`), else the first that can run. Choosing the organisation's default is written to the change record as `chat.default`.

- **A sign-in outlives a restart.** The key sign-ins are made with, and the sessions people ended, are kept in `.keeper.json` in the sessions folder (`HAAS_KEEPER` names another file), readable by the server's user alone. A deploy no longer signs everybody out. Deleting the file signs everybody out, which is how to do that on purpose. The desktop app stays signed in for 14 days (the front door's hour, behind one).
- **Every engine is offered every model.** Which pairs work well is for the person to find out. A credential's `harnesses` list, and the "works with" boxes on the Models page, are gone; a credential file or a registry that still has `harnesses` is read, and the list is ignored. A call to a key with a bearer token in Anthropic's Messages shape is translated into Chat Completions with no `wires` set, so Claude Code can use any OpenAI-compatible key.
- **The app's browser goes straight to the provider.** Signing in from the desktop app no longer stops at a page that asks for one more press.

**New**

- **A virtual machine for each job on a person's computer.** `haas local wall install` downloads OpenShell, checks it, signs the driver on a Mac, and keeps its gateway running under the name `haas`; haas then chooses the virtual machine wall by itself. It needs the release's `haas-rootfs-<arch>.tar.gz` (the runner image, flattened, for `amd64` and `arm64`), which the release workflow now builds and attaches with its checksum, and on a Mac the disk tools `mke2fs`, `debugfs` and `e2fsck` (`brew install e2fsprogs` until haas ships them). `HAAS_OPENSHELL_GATEWAY` names the gateway the virtual machine probe asks ([LOCAL.md](LOCAL.md#the-wall-between-an-agent-and-your-files)).
- **haas on a person's computer chooses its sandbox, and says which.** It looks for a virtual machine (OpenShell), then a container (Docker or Podman, with the runner image and a unix socket it can mount), then on a Mac the computer's own sandbox (Seatbelt: the job writes only its own folder, reads none of the rest of the home and reaches only haas's proxy), then nothing, and runs jobs behind the first it finds. The chat page and `haas doctor` say which and why not stronger; with nothing, the person confirms once (`sandbox.confirmed` in the change record). `HAAS_SANDBOX` still decides when set ([LOCAL.md](LOCAL.md#the-wall-between-an-agent-and-your-files)).
- **The desktop app runs haas on this computer.** With no organisation set, it uses haas as the install script left it, or starts the `haas` it now carries and stops it when it quits; its menu switches between this computer and an organisation. `haas local link` prints a sign-in link and opens nothing. On macOS, haas on a person's computer keeps its data in `~/Library/Application Support/haas/data`.
- **Engines and models on a person's own computer.** `haas local` finds each engine on the person's PATH and says how to install a missing one; the chat page asks for a model (a provider, a key, the models) until there is one, and keeps the key in the computer's keychain; `haas local use NAME` adds a key the person's shell has ([LOCAL.md](LOCAL.md#models-and-keys)). A provider in the registry can name the shapes of API it answers (`wires`), as a credential can.
- **haas on a person's own computer.** The install script starts haas as a service of the person's (`haas local install`) and opens it in their browser, signed in by a link that works once (`haas open`): no password, on `127.0.0.1` only, in a folder of their own ([LOCAL.md](LOCAL.md)). `HAAS_NO_START=1` installs without starting.
- **`haas` is a download.** Each release has the `haas` program for macOS, Linux and Windows on Intel and Arm, as `haas_<release>_<system>_<processor>.tar.gz` (a `.zip` on Windows), in the `SHA256SUMS` the release workflow signs. `deploy/server/build.sh` builds one.
- **The desktop app.** A release has the app for macOS (Apple silicon and Intel), Windows and Linux, and a server can hand it out so that people's apps update themselves: set `HAAS_DESKTOP` to a folder and fill it with `deploy/desktop/publish.sh` ([DESKTOP.md](DESKTOP.md#the-app)). Nothing changes for a server that sets nothing.
- **A folder for a chat.** A chat can work in a folder of the agent's computer in the cloud, chosen in an explorer, and every turn of it starts there (a job's optional `dir`, and a conversation's). In the desktop app a folder of the person's own computer can also be kept the same as an agent's folder by the helper ([DESKTOP.md](DESKTOP.md#working-in-a-folder)). Agents gain an optional `folder` key. Nothing changes for what has neither.
- **Presets in the chat.** A chat can start from a preset, from a menu in its box: one that comes built in (the skills of a ready-made kind of work) or one a person saved from a chat with its engine and model. The Presets page lists both. Saved presets are one file for each person in the new `presets` directory (`HAAS_PRESETS`, format 1, [UPGRADE.md](UPGRADE.md)). Nothing changes for a server that does not use them.
- **A chat page.** `/chat` opens on a box with the agent and model chosen under it, and every conversation shows the same list of the person's agents beside it.

## v0.0.1-alpha.5

Since `v0.0.1-alpha.4`. The data stays at format 1 in every store. Nothing to set and nothing new refused.

**New**

- **The helper is a download.** Each release has `haas-helper` for macOS, Linux and Windows on Intel and Arm, a `SHA256SUMS` the release workflow signs, and `install.sh` and `install.ps1`, which check the package against the sums and the sums' signature when cosign is installed ([DESKTOP.md](DESKTOP.md#installing-the-helper)). `haas-helper version` says which release it is.
- **`haas-helper sync`:** a folder on a person's computer kept the same as their agent's folder in the cloud, with a written rule for a file both sides changed ([DESKTOP.md](DESKTOP.md#files)). The server gains `PUT` and `DELETE` on `/api/instances/{id}/file` and `POST .../hashes` for a person's token, and the file listing says each file's time to the nanosecond.
- **`haas-helper acp`:** the helper as an agent for an editor that speaks the Agent Client Protocol ([DESKTOP.md](DESKTOP.md#editors)).

## v0.0.1-alpha.4

Since `v0.0.1-alpha.3`. Nothing to set and nothing new refused.

**Fixed**

- Stopping a job that runs in a container took the container's whole grace of sixty seconds and left no manifest, because the runner in the box cannot signal the agent's processes. The agent's processes are now ended as the agent, so a stop ends the run in about a second and the manifest says who stopped it.

## v0.0.1-alpha.3

Since `v0.0.1-alpha.2`. The data stays at format 1 in every store.

**Settings added**

- `HAAS_OPENSHELL_REMOTE=1`: the OpenShell gateway is on another machine, so this one needs no `/dev/kvm`.
- The chart's `auth.oidc` and `spend` values, and the OIDC settings in `deploy/compose/env.example`. The server already took these settings; [SSO.md](SSO.md) now says what they do.

**New**

- **Stop a turn:** a Stop button on the conversation and `POST /api/jobs/{id}/stop`. A stopped turn ends `interrupted`, and its manifest has a new `stoppedBy` field (`schema/manifest/v1.json`). A harness that is stopped, or whose time runs out, has its whole process group killed, so what it started cannot hold the run open. Containers are named `haas-<job id>` ([STATES.md](STATES.md#stopping-a-job)).
- **A person's computer:** `haas-helper login`, `whoami`, `logout`, `run`, `status` and `service`. With `HAAS_API_TOKENS` set, a computer signs in by the device flow and holds a token that acts as one person, with the one scope `work`, which no admin form grants. It keeps the token in the operating system's keychain, follows the person's jobs, and starts at login ([TOKENS.md](TOKENS.md), [DESKTOP.md](DESKTOP.md)). `haas` has `login`, `whoami` and `logout` too, and now builds for Windows.
- **A person's token reads and acts as JSON:** it lists their agents and the models they may use, starts an agent, opens a thread and says something in it, reads or follows a turn, and hears `GET /api/events` ([TOKENS.md](TOKENS.md)).
- **`Idempotency-Key`** on `POST /api/jobs` and on a thread's `say`: a send made twice with one key is one job.

**Refused now**

- `HAAS_SANDBOX=openshell` on a Linux machine with no `/dev/kvm`. The server does not start, and `haas doctor` reports it.
- Starting an agent this server does not offer or cannot run, which the page used to accept and fail later.

## v0.0.1-alpha.2

Since `v0.0.1-alpha.1`. Nothing to set and nothing new refused.

**Fixed**

- A server that held credentials refused every job that did not name one, including a job whose harness needs no model. `deploy/smoke.sh` failed on such an install.

## v0.0.1-alpha.1

Since `v0.1.0-rc.1`. The data stays at format 1 in every store, and
`haas data rollback -to v0.1.0-rc.1` needs nothing undone.

**Settings added**

- `HAAS_LOCAL_REPOS`: the directories a job's repository may be cloned from. Unset, a repository is an https or ssh address and nothing on the server's own disk. **A deployment that submitted local paths has to set it.**
- `HAAS_RUNNER_GROUPS`, `HAAS_ADMIN_GROUPS`, `HAAS_ADMINS`, `HAAS_OIDC_SCOPES`, `HAAS_OIDC_GROUPS_CLAIM`: roles from the identity provider's groups.
- `HAAS_API_TOKENS` and `HAAS_TOKEN`: scoped, revocable API tokens. With the file set, the API refuses the shared password ([TOKENS.md](TOKENS.md)).
- `HAAS_LIMIT_PERSON`, `HAAS_LIMIT_TEAM`: the most one person or team runs at once ([LIMITS.md](LIMITS.md)).
- `HAAS_RETENTION`: removes finished work on a schedule. With no policy nothing is removed ([RETENTION.md](RETENTION.md)).
- `HAAS_CHANGES`: where the record of what changed on the platform is kept ([CHANGES.md](CHANGES.md)).
- `HAAS_MIGRATE_IRREVERSIBLE`: lets one start run a migration that cannot be undone.
- `HAAS_DEPLOY_RESULT`: a file a deploy writes, reported at `/metrics`.
- `HAAS_SANDBOX=openshell` and `HAAS_OPENSHELL*`: run jobs in OpenShell sandboxes ([OPENSHELL.md](OPENSHELL.md)).
- `HAAS_GITHUB_APP_ID`, `HAAS_GITHUB_APP_KEY`, `HAAS_GITHUB_URL`: a job reaches a private repository with a token for it alone.
- `HAAS_REGISTRY`, `HAAS_ORG_SETTINGS`: an organisation's model registry and settings, set by a front door.
- A credential's `wires` and `maxOutput`: a call in a shape the provider does not answer is translated to Chat Completions ([GATEWAY.md](GATEWAY.md)).
- `HAAS_SIGNING_KEY` takes `awskms://<key>`.

**Defaults changed**

- A job's repository is an https or ssh address, or a directory under `HAAS_LOCAL_REPOS`. It is no longer any path on the server.
- `/readyz` answers 503 when fewer workers run than `HAAS_JOB_CONCURRENCY`, or the jobs directory cannot be written ([RUNBOOK.md](RUNBOOK.md)).

**Refused now**

- A server with an identity provider and neither a group mapping nor `HAAS_ALLOWED` does not start.
- A person the provider signs in whom no group maps to is sent back to the login page.
- Only an admin changes setups and skills.
- A migration that cannot be undone, unless `HAAS_MIGRATE_IRREVERSIBLE=1`.
