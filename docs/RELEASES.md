# What each release changes for an operator

A release's notes list the images and the merged pull requests. This file lists what an operator needs before upgrading: settings added, defaults changed, and what the server now refuses.

A tag needs a section here named for it, or the release workflow stops and copies the section into the release's notes. Changes made after a release go under **Next release**, renamed to the tag when the tag is made. See [UPGRADE.md](UPGRADE.md) for the data and [PILOT.md](PILOT.md) for installing and checking a release.

## Next release

Nothing yet.

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
