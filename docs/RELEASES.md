# What each release changes for an operator

A release's notes list the images and the merged pull requests. This file lists what an operator needs before upgrading: settings added, defaults changed, and what the server now refuses.

A tag needs a section here named for it, or the release workflow stops and copies the section into the release's notes. Changes made after a release go under **Next release**, renamed to the tag when the tag is made. See [UPGRADE.md](UPGRADE.md) for the data and [PILOT.md](PILOT.md) for installing and checking a release.

## Next release

**Added**

- `GUSTO_CLOUD_SERVERS=shared` runs every organization's server in one process on this machine, in place of a process each. Each server keeps its own ports, data, keys and sign-in, and is started from the same settings as before. The process is `gusto shared-servers`, which gusto cloud starts with the `GUSTO_CLOUD_ORG_ENV` settings and starts again if it ends; its output and every server's is in `shared-servers/host.log` under `GUSTO_CLOUD_ROOT`. It cannot be combined with `GUSTO_CLOUD_NODES_DIR`. `on`, a process each, stays the default.
- Send now: a message sent while a reply is being written stops that reply and runs next, in the same session, so the agent keeps what it did. The message box's Send does this while a reply runs; the API takes `"now": true` on `POST /api/instances/{id}/threads/{thread}/say`.
- Stopping a turn of an agent that speaks ACP (Claude Code, Codex, DeepSeek Agent, Hermes) sends it `session/cancel` and lets it end the turn itself, for up to 15 seconds before it is killed. Before, it was killed at once.

- `gusto cloud migrate -copy-from <cloud.db>` copies the accounts in a SQLite file into the Postgres database `GUSTO_DATABASE_URL` names, in one transaction. Times and booleans are given as Postgres's columns take them, and tables go in the order their foreign keys need. The file is only read, and a Postgres schema that holds accounts already is refused.
- A host started with `DATABASE=postgres` runs Postgres itself, reached only by its socket, and keeps gusto cloud's accounts in its `cloud` schema and each organization's indexes in a schema of the organization's own. A host that kept its accounts in `cloud.db` has them copied in once, before gusto cloud first starts on Postgres; the file is left as it was. The database is dumped to the bucket every hour, under `backups/postgres/`, and the bucket keeps each dump for 30 days.

**Changed**

- OpenCode runs over ACP (`opencode acp`) in place of `opencode run --format json`, so it takes Send now and Stop as the other ACP agents do: asked to cancel, keeping its session. Its model is set by `model` in its config, since `opencode acp` takes no `--model`.
- cloud deploy builds the x86 runner image, which only `cloud up`'s hosts need, in a job of its own on one of GitHub's x86 runners, free for a public repository, and the Arm image on the Mac runner. A production deploy does not wait for the x86 image, and `cloud up` refuses to start without it.

**Removed**

- gusto cloud no longer starts and stops nodes on EC2. `GUSTO_CLOUD_NODE_LAUNCH`, `GUSTO_CLOUD_NODE_PROJECT`, `GUSTO_CLOUD_NODE_TEMPLATE`, `GUSTO_CLOUD_NODE_USERDATA`, `GUSTO_CLOUD_NODE_SETTINGS`, `GUSTO_CLOUD_NODE_IMAGE_PARAMETER`, `GUSTO_CLOUD_NODE_MAX`, `GUSTO_CLOUD_NODE_IDLE_MINUTES`, `GUSTO_CLOUD_NODE_PREWARMS` and `GUSTO_CLOUD_NODE_KEEP` are refused by name, by `gusto cloud` and `gusto operator`. Nodes started another way still register in `GUSTO_CLOUD_NODES_DIR`. The demo form no longer asks for a node as the visitor types (`POST /demo/warm` is gone), and the operator's page no longer lists hosts.

## v0.1.1

**Changed**

- Where agents run in rootless podman, gusto-boot, the upgrade and each organization's server give podman the user's session bus (`DBUS_SESSION_BUS_ADDRESS`, `XDG_RUNTIME_DIR`) and one image store (`CONTAINERS_STORAGE_CONF`). A build or a container with limits failed without the bus, and a server, whose `HOME` is its organization's folder, would not have found the runner image. Hosts with Docker are unchanged.
- `deploy/greencloud/make-vault.sh` reads Parameter Store in tens, the most it answers at once.

## v0.1.0

**Added**

- `gusto param` reads a parameter from a file under `GUSTO_PARAM_DIR` when it is set, at the parameter's own name, such as `GUSTO_PARAM_DIR/gusto/stripe-key`. A host off AWS keeps its secrets so ([deploy/greencloud](../deploy/greencloud/README.md)).
- The gusto cloud landing page takes a note to the team: a work email, a company and what they want to talk about. It is mailed to `GUSTO_CLOUD_DEMO_NOTIFY` and listed on the operator's page ([CLOUD.md](CLOUD.md)). The accounts database moves to schema 9, which adds the `contacts` table; an earlier release does not open a database at schema 9.
- Where agents run in containers on Linux, a person can share an app with a command of its own. It runs in a container of its own with no network, which the server reaches through a unix socket in `GUSTO_APP_SOCKETS` (by default `app-sockets` in `GUSTO_DATA`; on gusto cloud, the node's own disk). A person's kept home is not reset or removed while an app they shared runs on it ([CLOUD.md](CLOUD.md)). It needs a runner image from this release, which carries `gusto inside-app`.
- An organization in gusto cloud has teams, such as Engineering and Business. Each holds its own tasks and records, conversations, agents, connections to apps and a computer for each of its people, which only its members reach ([CLOUD.md](CLOUD.md#teams)). Every organization has the team `main`, which holds everything and everybody it had before, so nobody sees a change until an admin makes a second one at `/org`.
- A person's kept home in a container has a size limit, `GUSTO_HOME_LIMIT_MB` (5120 by default). A turn over it is refused until the person resets the home on Connections. A turn that ended for that reason says so on its page, and a reset is recorded as `home.reset` ([CLOUD.md](CLOUD.md), [CHANGES.md](CHANGES.md)).
- The accounts database is at schema 8 (`workspaces` and `workspace_members`, which hold the teams, and the team an invitation's link joins), and the task index at schema 4 (the team a task is in, in the `team` column, which held a spending label before; the agent's turn columns `agent`, `handed_by`, `answers`, `hops` and `paid_by`; and `progress`, which orders a task's states). Both migrate on start. The task index never writes a row over one that is further along, so `gusto tasks reindex` leaves a row that is ahead of its `state.json`. To make the files win after they were restored from a backup, stop the server and use `gusto tasks reindex -from-scratch`.
- `job.json` keeps the team a task is in as `workspace`, and a spec sent to `POST /api/jobs` may name it as `team` too. Its schema now also describes `person`, `agent` and `dir`, which tasks already carried.
- gusto cloud serves each organization at `<GUSTO_CLOUD_URL>/workspace/<slug>` (`GUSTO_CLOUD_ORGS_AT`, `path` by default; `host` serves it at `<slug>.<GUSTO_CLOUD_DOMAIN>`). At a path the organization shares gusto cloud's one sign-in, and an old host name sends a link on to the path ([CLOUD.md](CLOUD.md#where-an-organization-is)).
- A person who signs up with a personal address gets a workspace of their own, named for them, without a form to fill in.
- `GUSTO_CLOUD_APPS_DOMAIN` is the domain shared apps are served under, `GUSTO_CLOUD_DOMAIN` by default. Give it a domain of its own, such as `example-apps.com`, with a wildcard name and certificate. The organization's server is told it as `GUSTO_APPS_URL`. A demo host takes it as `GUSTO_CLOUD_APPS_DOMAIN` and `GUSTO_CLOUD_APPS_ZONE`.
- Every gusto command reads its settings from a TOML file named by `GUSTO_SETTINGS_FILE` too. A key is the setting's name without `GUSTO_`, in lower case, and the environment is read over the file. A secret is read from the environment only, so the file is refused when it holds one ([REFERENCE.md](REFERENCE.md#settings-file)). Nothing changes for a deployment that sets no file.

**Changed**

- A gusto cloud node tells gusto cloud when it starts, at `<GUSTO_CLOUD_URL>/nodes/started`, and an organization waiting for room is placed on it at once rather than at the next look every half second. A node must reach gusto cloud's public address; nodes already do, for their models. A page waiting on an organization's server, such as a demo's wait page, is drawn again when the server's state changes, not every second ([CLOUD.md](CLOUD.md)).
- An organization's pages at `/workspace/<slug>` load the app's files and `/static/` from under that path, from the organization's own server. During a deploy, a page no longer loads blank while gusto cloud is already on the new release and the organization's server is not yet.
- An organization's server behind gusto cloud has connector sign-ins come back to `<GUSTO_CLOUD_URL>/connections/callback`, which hands each on to its organization by the sign-in's `state`. A GitHub, Asana or HubSpot client is registered with that one address ([GATEWAY.md](GATEWAY.md)).
- gusto cloud starts an organization's server with its settings in `orgs/<slug>/server.toml` and only its secrets in its environment ([CLOUD.md](CLOUD.md)). The lines of `GUSTO_CLOUD_ORG_ENV` and `GUSTO_CLOUD_DEMO_ENV` go the same way: a setting into the file, a secret into the environment.
- One setting, `GUSTO_DATA`, names the folder gusto keeps its data in, the current directory by default. Each store is a fixed folder of it: `jobs`, `sessions`, `setups`, `comparisons`, `integrations`, `presets`, `crew`, `changes/changes.jsonl`, `apps`, `outbox`, `agents`, and `sessions/.keeper.json` for sign-ins. `GUSTO_TASKS`, `GUSTO_SESSIONS`, `GUSTO_SETUPS`, `GUSTO_COMPARISONS`, `GUSTO_INTEGRATIONS`, `GUSTO_PRESETS`, `GUSTO_CREW`, `GUSTO_CHANGES`, `GUSTO_APPS`, `GUSTO_MAIL_OUTBOX`, `GUSTO_KEEPER` and `GUSTO_AGENTS` are refused, each saying to set `GUSTO_DATA`. The compose file, the Helm chart and gusto cloud already kept every store under one folder, and set `GUSTO_DATA` now. An install that points one of these settings elsewhere moves that folder under `GUSTO_DATA` before upgrading.
- Settings named after the old words are refused, and the server says the new name ([TERMS.md](TERMS.md)). Rename them before upgrading:

  | Was | Is |
  | --- | --- |
  | `GUSTO_JOBS` | `GUSTO_DATA`, whose `jobs` folder holds the tasks |
  | `GUSTO_JOB_NETWORK`, `GUSTO_JOB_CONCURRENCY`, `GUSTO_JOB_CPUS`, `GUSTO_JOB_MEMORY` | `GUSTO_TASK_NETWORK`, `GUSTO_TASK_CONCURRENCY`, `GUSTO_TASK_CPUS`, `GUSTO_TASK_MEMORY` |
  | `GUSTO_CLOUD_JOBS`, `GUSTO_CLOUD_TRIAL_JOBS`, `GUSTO_CLOUD_PAID_JOBS_PER_SEAT` | `GUSTO_CLOUD_TASKS`, `GUSTO_CLOUD_TRIAL_TASKS`, `GUSTO_CLOUD_PAID_TASKS_PER_SEAT` |
  | `GUSTO_CELL`, and each `GUSTO_CELL_*` and `GUSTO_CELLS_DIR` | `GUSTO_NODE`, `GUSTO_NODE_*` and `GUSTO_NODES_DIR` |
  | each `GUSTO_CLOUD_CELL_*`, `GUSTO_CLOUD_CELLS_DIR` and `GUSTO_CLOUD_LOCAL_CELL` | `GUSTO_CLOUD_NODE_*`, `GUSTO_CLOUD_NODES_DIR` and `GUSTO_CLOUD_LOCAL_NODE` |
  | `GUSTO_OWNER_ADDR` | `GUSTO_OPERATOR_ADDR` |
  | `GUSTO_FRONT_DOOR` | `GUSTO_CLOUD_URL` |

- A task's spending counts against the team it is in. `team` in a `job.json` written before, a label the person typed, is read for nothing: the team limit (`GUSTO_LIMIT_TEAM`), a team's cap in the spend policy and the spending report go by team id, such as `main`. Name teams by id in `GUSTO_SPEND_POLICY` before upgrading. The task form has no team field, and a spec sent to `POST /api/jobs` naming a team the sender is not in is refused.
- `gusto host` is `gusto node`, and `gusto cloud workspaces` is `gusto cloud computers`. The old names are gone.
- Ending a sandbox, or stopping one, waits at most two minutes for the OpenShell gateway, so a gateway that does not answer no longer holds a turn's worker. Ending a sandbox twice, or one that was never made, reports nothing.

## v0.1.0-rc.2

**Changed**

- opencode uses the task's own model from `GUSTO_MODEL_PROVIDER` and `GUSTO_MODEL_ID`. Agent configuration moved from `image/harness/` to `internal/harness/config/`; an image built from this repository copies it from there.
- `install.sh` and `install.ps1` install `gusto` and `gusto-helper` (`GUSTO_PROGRAMS` names fewer) and live in `deploy/install/`. They install to `~/.local/bin` (`GUSTO_INSTALL_DIR` changes it; `GUSTO_HELPER_DIR` is still read) or, on Windows, `%LOCALAPPDATA%\gusto`, not `%LOCALAPPDATA%\gusto-helper`: an earlier helper stays in the old folder and on the PATH until removed.
- Every agent is offered every model. A credential's `harnesses` list and the Models page's "works with" boxes are gone, and a file that still has `harnesses` is read with the list ignored. A bearer-token call in Anthropic's Messages shape is translated to Chat Completions when no `wires` is set.
- A new chat starts on the person's last choice, else the organization's default (`/admin`, New chats; recorded as `chat.default`), else `GUSTO_CHAT_DEFAULT` (`agent=model`), else the first that can run.
- Sign-ins survive a restart. The sign-in key and ended sessions are kept in `.keeper.json` in the sessions folder (`GUSTO_KEEPER` names another file). Delete the file to sign everybody out. The desktop app stays signed in for 14 days.

**New**

- `deploy/install/server.sh` sets up a Linux machine in one command: Docker, a password, gusto, and with `--domain` HTTPS through the `tls` profile (Caddy). It checks that ports 80 and 443 are free, and turns on `GUSTO_API_TOKENS`, so the API refuses the shared password ([TOKENS.md](TOKENS.md)).
- `server.sh --behind-proxy` keeps gusto on `127.0.0.1:8040` for nginx or Apache. `GUSTO_BEHIND_PROXY=1` makes the sign-in lock, the record and the access log use the last forwarded address ([compose/README.md](../deploy/compose/README.md#behind-your-own-web-server)).
- With no identity provider, the shared password signs in the one owner, who can approve a computer by the device flow (`gusto-helper login`, needs `GUSTO_API_TOKENS`). With one, the shared sign-in cannot ([TOKENS.md](TOKENS.md)).
- gusto on a person's computer runs as a service the install script starts, opened by `gusto open` on `127.0.0.1` only. `GUSTO_NO_START=1` installs without starting. `gusto local use NAME` adds a key from the shell ([LOCAL.md](LOCAL.md), [models](LOCAL.md#models-and-keys)). The desktop app uses it when no organization is set. `gusto local link` prints a sign-in link and opens nothing.
- gusto picks a virtual machine, a container, a Mac's Seatbelt sandbox, a Linux Landlock sandbox (6.7 or later, through `gusto sandbox-run`), or nothing, in that order. `gusto doctor` says which. `GUSTO_SANDBOX` still decides when set ([LOCAL.md](LOCAL.md#the-sandbox-between-an-agent-and-your-files)).
- `gusto local sandbox install` sets up the virtual machine sandbox (OpenShell). It needs the release's `gusto-rootfs-<arch>.tar.gz` (`amd64`, `arm64`) and, on a Mac, `mke2fs`, `debugfs` and `e2fsck` (`gusto-e2fsprogs-darwin-arm64.tar.gz`, built by `deploy/e2fsprogs/build.sh`). `GUSTO_OPENSHELL_GATEWAY` names the gateway the probe asks.
- Each release carries `gusto` for macOS, Linux and Windows on Intel and Arm (`gusto_<release>_<system>_<processor>.tar.gz`, `.zip` on Windows). `deploy/server/build.sh` builds one.
- A server hands out the desktop app when `GUSTO_DESKTOP` names a folder filled by `deploy/desktop/publish.sh` ([DESKTOP.md](DESKTOP.md#the-app)).
- A chat can work in a folder of the person's workspace in the cloud (a task's optional `dir`) and sync a local folder with it ([DESKTOP.md](DESKTOP.md#working-in-a-folder)). Agents gain an optional `folder` key.
- A chat can start from a preset, built in or saved. Saved presets are one file each in the `presets` directory (`GUSTO_PRESETS`, format 1, [UPGRADE.md](UPGRADE.md)).
