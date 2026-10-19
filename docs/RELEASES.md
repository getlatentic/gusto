# What each release changes for an operator

A release's notes list the images and the merged pull requests. This file lists what an operator needs before upgrading: settings added, defaults changed, and what the server now refuses.

A tag needs a section here named for it, or the release workflow stops and copies the section into the release's notes. Changes made after a release go under **Next release**, renamed to the tag when the tag is made. See [UPGRADE.md](UPGRADE.md) for the data and [PILOT.md](PILOT.md) for installing and checking a release.

## Next release

**Changed**

- opencode uses the task's own model from `GUSTO_MODEL_PROVIDER` and `GUSTO_MODEL_ID`. Engine configuration moved from `image/harness/` to `internal/harness/config/`; an image built from this repository copies it from there.
- `install.sh` and `install.ps1` install `gusto` and `gusto-helper` (`GUSTO_PROGRAMS` names fewer) and live in `deploy/install/`. They install to `~/.local/bin` (`GUSTO_INSTALL_DIR` changes it; `GUSTO_HELPER_DIR` is still read) or, on Windows, `%LOCALAPPDATA%\gusto`, not `%LOCALAPPDATA%\gusto-helper`: an earlier helper stays in the old folder and on the PATH until removed.
- Every engine is offered every model. A credential's `harnesses` list and the Models page's "works with" boxes are gone, and a file that still has `harnesses` is read with the list ignored. A bearer-token call in Anthropic's Messages shape is translated to Chat Completions when no `wires` is set.
- A new chat starts on the person's last choice, else the organisation's default (`/admin`, New chats; recorded as `chat.default`), else `GUSTO_CHAT_DEFAULT` (`engine=model`), else the first that can run.
- Sign-ins survive a restart. The sign-in key and ended sessions are kept in `.keeper.json` in the sessions folder (`GUSTO_KEEPER` names another file). Delete the file to sign everybody out. The desktop app stays signed in for 14 days.

**New**

- `deploy/install/server.sh` sets up a Linux machine in one command: Docker, a password, gusto, and with `--domain` HTTPS through the `tls` profile (Caddy). It checks that ports 80 and 443 are free, and turns on `GUSTO_API_TOKENS`, so the API refuses the shared password ([TOKENS.md](TOKENS.md)).
- `server.sh --behind-proxy` keeps gusto on `127.0.0.1:8040` for nginx or Apache. `GUSTO_BEHIND_PROXY=1` makes the sign-in lock, the record and the access log use the last forwarded address ([compose/README.md](../deploy/compose/README.md#behind-your-own-web-server)).
- With no identity provider, the shared password signs in the one owner, who can approve a computer by the device flow (`gusto-helper login`, needs `GUSTO_API_TOKENS`). With one, the shared sign-in cannot ([TOKENS.md](TOKENS.md)).
- gusto on a person's computer runs as a service the install script starts, opened by `gusto open` on `127.0.0.1` only. `GUSTO_NO_START=1` installs without starting. `gusto local use NAME` adds a key from the shell ([LOCAL.md](LOCAL.md), [models](LOCAL.md#models-and-keys)). The desktop app uses it when no organisation is set. `gusto local link` prints a sign-in link and opens nothing.
- gusto picks a virtual machine, a container, a Mac's Seatbelt sandbox, a Linux Landlock sandbox (6.7 or later, through `gusto sandbox-run`), or nothing, in that order. `gusto doctor` says which. `GUSTO_SANDBOX` still decides when set ([LOCAL.md](LOCAL.md#the-sandbox-between-an-agent-and-your-files)).
- `gusto local sandbox install` sets up the virtual machine sandbox (OpenShell). It needs the release's `gusto-rootfs-<arch>.tar.gz` (`amd64`, `arm64`) and, on a Mac, `mke2fs`, `debugfs` and `e2fsck` (`gusto-e2fsprogs-darwin-arm64.tar.gz`, built by `deploy/e2fsprogs/build.sh`). `GUSTO_OPENSHELL_GATEWAY` names the gateway the probe asks.
- Each release carries `gusto` for macOS, Linux and Windows on Intel and Arm (`gusto_<release>_<system>_<processor>.tar.gz`, `.zip` on Windows). `deploy/server/build.sh` builds one.
- A server hands out the desktop app when `GUSTO_DESKTOP` names a folder filled by `deploy/desktop/publish.sh` ([DESKTOP.md](DESKTOP.md#the-app)).
- A chat can work in a folder of the person's workspace in the cloud (a task's optional `dir`) and sync a local folder with it ([DESKTOP.md](DESKTOP.md#working-in-a-folder)). Agents gain an optional `folder` key.
- A chat can start from a preset, built in or saved. Saved presets are one file each in the `presets` directory (`GUSTO_PRESETS`, format 1, [UPGRADE.md](UPGRADE.md)).

## v0.0.1-alpha.5

The data stays at format 1 in every store. Nothing to set and nothing new refused.

- `gusto-helper` is a download with signed `SHA256SUMS`, `install.sh` and `install.ps1` ([DESKTOP.md](DESKTOP.md#installing-the-helper)).
- `gusto-helper sync` mirrors a local folder with the agent's cloud folder ([DESKTOP.md](DESKTOP.md#files)). The server gains `PUT` and `DELETE` on `/api/instances/{id}/file` and `POST .../hashes` for a person's token.
- `gusto-helper acp` is an agent for editors that speak the Agent Client Protocol ([DESKTOP.md](DESKTOP.md#editors)).

## v0.0.1-alpha.3

The data stays at format 1 in every store.

- Added `GUSTO_OPENSHELL_REMOTE=1`: the OpenShell gateway is on another machine, so this one needs no `/dev/kvm`. Added the chart's `auth.oidc` and `spend` values and the OIDC settings in `deploy/compose/env.example` ([SSO.md](SSO.md)).
- Added a Stop button and `POST /api/jobs/{id}/stop`. A stopped turn ends `interrupted`, and its manifest has a `stoppedBy` field ([STATES.md](STATES.md#stopping-a-task)).
- Added `gusto-helper login`, `whoami`, `logout`, `run`, `status` and `service`. With `GUSTO_API_TOKENS` set, a computer signs in by the device flow and holds a token for one person with the scope `work`, which reads and acts as JSON ([TOKENS.md](TOKENS.md)).
- Added `Idempotency-Key` on `POST /api/jobs` and on a thread's `say`: a send made twice with one key is one task.
- Refused now: `GUSTO_SANDBOX=openshell` on a Linux machine with no `/dev/kvm` (the server does not start, and `gusto doctor` reports it), and starting an agent the server does not offer or cannot run.

## v0.0.1-alpha.1

The data stays at format 1 in every store, and `gusto data rollback -to v0.1.0-rc.1` needs nothing undone.

**Settings added**

- `GUSTO_LOCAL_REPOS`: the directories a task's repository may be cloned from. **A deployment that submitted local paths has to set it.**
- `GUSTO_RUNNER_GROUPS`, `GUSTO_ADMIN_GROUPS`, `GUSTO_ADMINS`, `GUSTO_OIDC_SCOPES`, `GUSTO_OIDC_GROUPS_CLAIM`: roles from the identity provider's groups.
- `GUSTO_API_TOKENS` and `GUSTO_TOKEN`: scoped, revocable API tokens. With the file set, the API refuses the shared password ([TOKENS.md](TOKENS.md)).
- `GUSTO_LIMIT_PERSON`, `GUSTO_LIMIT_TEAM`: the most one person or team runs at once ([LIMITS.md](LIMITS.md)).
- `GUSTO_RETENTION`: removes finished work on a schedule ([RETENTION.md](RETENTION.md)). `GUSTO_CHANGES`: where the change record is kept ([CHANGES.md](CHANGES.md)).
- `GUSTO_MIGRATE_IRREVERSIBLE`: lets one start run a migration that cannot be undone. `GUSTO_DEPLOY_RESULT`: a file a deploy writes, reported at `/metrics`.
- `GUSTO_SANDBOX=openshell` and `GUSTO_OPENSHELL*`: run tasks in OpenShell sandboxes ([OPENSHELL.md](OPENSHELL.md)).
- `GUSTO_GITHUB_APP_ID`, `GUSTO_GITHUB_APP_KEY`, `GUSTO_GITHUB_URL`: a task reaches a private repository with a token for it alone.
- `GUSTO_REGISTRY`, `GUSTO_ORG_SETTINGS`: an organisation's model registry and settings. A credential's `wires` and `maxOutput` translate calls to Chat Completions ([GATEWAY.md](GATEWAY.md)). `GUSTO_SIGNING_KEY` takes `awskms://<key>`.

**Defaults changed and refused**

- A task's repository is an https or ssh address, or a directory under `GUSTO_LOCAL_REPOS`, not any path on the server.
- `/readyz` answers 503 when fewer workers run than `GUSTO_JOB_CONCURRENCY`, or the tasks directory cannot be written ([RUNBOOK.md](RUNBOOK.md)).
- A server with an identity provider and neither a group mapping nor `GUSTO_ALLOWED` does not start.
- A person the provider signs in whom no group maps to is sent back to the login page. Only an admin changes setups and skills.
- A migration that cannot be undone is refused, unless `GUSTO_MIGRATE_IRREVERSIBLE=1`.
