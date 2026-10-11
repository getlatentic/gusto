# haas on a person's own computer

One person runs haas on their own computer, where the computer is theirs and the agents work on its folders. It is the same
server as a shared one, started for one person, on the loopback, in a folder of their own.

## Install

On macOS and Linux:

```bash
curl -fsSL https://github.com/getlatentic/haas/releases/latest/download/install.sh | sh
```

On Windows, in PowerShell. Windows refuses to run a downloaded script by default, so the script is read and run in one step:

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://github.com/getlatentic/haas/releases/latest/download/install.ps1 | iex"
```

The script installs `haas` and `haas-helper` (deploy/install/install.sh, [DESKTOP.md](DESKTOP.md#installing-the-helper)) and then
runs `haas local install`, which makes haas a service of this person's that starts at login, starts it, and opens it in the
browser signed in. `HAAS_NO_START=1` installs without starting. While the releases are private, the script needs the GitHub
command line, signed in.

## What it keeps

| What | Where |
| --- | --- |
| The data: jobs, conversations, presets, records | macOS `~/Library/Application Support/haas`, Linux `$XDG_DATA_HOME/haas` or `~/.local/share/haas`, Windows `%LOCALAPPDATA%\haas\data`. `HAAS_LOCAL_HOME` names another folder |
| `local.json`: the port and the secret | in that folder, readable by this person alone. The secret is the server's password and the key its sign-in links are made with |
| The service | macOS a launch agent, `~/Library/LaunchAgents/com.latentic.haas.plist`. Linux a systemd user unit, `~/.config/systemd/user/haas.service`. Windows a task, `Latentic haas`, run at logon |
| The log | `haas.log` in the data folder on macOS and Windows; the journal on Linux |

It listens on `127.0.0.1`, on port 7140 when that is free the first time, and refuses to start on any other address.

## Signing in

There is no password page to fill in. `haas open` makes a link with the secret, which signs in once within five minutes, and
opens it. Another user of the same computer can reach the port but has no secret, so cannot sign in.

## Commands

```bash
haas open              # open it in the browser, signed in
haas local install     # make it a service that starts at login, start it, open it
haas local run         # what the service runs: the server, in the foreground
haas local stop        # stop it until the next login
haas local uninstall   # remove the service, and say where the data folder stays
```

## Testing an install without a release

`deploy/install/test-release.sh` builds haas from a checkout into a folder laid out as a release, with its checksums and the
install scripts. Serve the folder and point `HAAS_RELEASE_URL` at it to run the real install on a clean machine:

```bash
deploy/install/test-release.sh v0.0.0-test dist/test-release linux/arm64
python3 -m http.server -d dist/test-release 8000
```

On the test machine:

```bash
curl -fsSL http://<this machine>:8000/install.sh | HAAS_RELEASE_URL=http://<this machine>:8000 HAAS_PROGRAMS=haas sh -s v0.0.0-test
```

The build is not signed, so a machine with cosign installed refuses it.

## Not yet

- Jobs run in the server's own process (`HAAS_SANDBOX=here`): through the proxy, with none of the server's secrets, and with no
  walls around the files or the network (deploy/README.md). Choosing the strongest sandbox the machine has is
  getlatentic/planning#653.
- It finds no engines or model keys by itself: a person names their keys as a shared server's operator does. Finding the ones
  already on the machine is getlatentic/planning#656.
