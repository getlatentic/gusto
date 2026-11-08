# What each release changes for an operator

A release's notes list the images and the merged pull requests. This file lists what an operator needs before upgrading: settings added, defaults changed, and what the server now refuses.

A tag needs a section here named for it, or the release workflow stops and copies the section into the release's notes. Changes made after a release go under **Next release**, renamed to the tag when the tag is made. See [UPGRADE.md](UPGRADE.md) for the data and [PILOT.md](PILOT.md) for installing and checking a release.

## Next release

**Added**

- An organization in gusto cloud has workspaces, such as Engineering and Business. Each holds its own tasks and records, conversations, agents, connections to apps and a computer for each of its people, which only its members reach ([CLOUD.md](CLOUD.md#workspaces)). Every organization has the workspace `main`, which holds everything and everybody it had before, so nobody sees a change until an admin makes a second one at `/org`.
- A person's kept home in a container has a size limit, `GUSTO_HOME_LIMIT_MB` (5120 by default). A turn over it is refused until the person resets the home on Connections. A turn that ended for that reason says so on its page, and a reset is recorded as `home.reset` ([CLOUD.md](CLOUD.md), [CHANGES.md](CHANGES.md)).
- The accounts database is at schema 8 (`workspaces`, `workspace_members`, and the workspace an invitation's link joins), and the task index at schema 3 (a `workspace` column; the agent's turn columns `agent`, `handed_by`, `answers`, `hops` and `paid_by`; and `progress`, which orders a task's states). Both migrate on start. The task index never writes a row over one that is further along, so `gusto tasks reindex` leaves a row that is ahead of its `state.json`. To make the files win after they were restored from a backup, stop the server and use `gusto tasks reindex -from-scratch`.
- `job.json` may name its `workspace`. Its schema now also describes `person`, `agent` and `dir`, which tasks already carried.

**Changed**

- Settings named after the old words are refused, and the server says the new name ([TERMS.md](TERMS.md)). Rename them before upgrading:

  | Was | Is |
  | --- | --- |
  | `GUSTO_JOBS` | `GUSTO_TASKS` |
  | `GUSTO_JOB_NETWORK`, `GUSTO_JOB_CONCURRENCY`, `GUSTO_JOB_CPUS`, `GUSTO_JOB_MEMORY` | `GUSTO_TASK_NETWORK`, `GUSTO_TASK_CONCURRENCY`, `GUSTO_TASK_CPUS`, `GUSTO_TASK_MEMORY` |
  | `GUSTO_CLOUD_JOBS`, `GUSTO_CLOUD_TRIAL_JOBS`, `GUSTO_CLOUD_PAID_JOBS_PER_SEAT` | `GUSTO_CLOUD_TASKS`, `GUSTO_CLOUD_TRIAL_TASKS`, `GUSTO_CLOUD_PAID_TASKS_PER_SEAT` |
  | `GUSTO_CELL`, and each `GUSTO_CELL_*` and `GUSTO_CELLS_DIR` | `GUSTO_NODE`, `GUSTO_NODE_*` and `GUSTO_NODES_DIR` |
  | each `GUSTO_CLOUD_CELL_*`, `GUSTO_CLOUD_CELLS_DIR` and `GUSTO_CLOUD_LOCAL_CELL` | `GUSTO_CLOUD_NODE_*`, `GUSTO_CLOUD_NODES_DIR` and `GUSTO_CLOUD_LOCAL_NODE` |
  | `GUSTO_OWNER_ADDR` | `GUSTO_OPERATOR_ADDR` |
  | `GUSTO_FRONT_DOOR` | `GUSTO_CLOUD_URL` |

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
