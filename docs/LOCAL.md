# haas on a person's own computer

One person runs haas on their own computer, where the computer is theirs and the agents work on its folders. It is the same
server as a shared one, started for one person, on the loopback, in a folder of their own.

## Install

```bash
curl -fsSL https://github.com/getlatentic/haas/releases/latest/download/install.sh | sh
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
| The log | `haas.log` in the data folder on macOS; the journal on Linux |

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
haas local uninstall   # remove the service; the data folder stays
```

## Not yet

- Jobs run in the server's own process (`HAAS_SANDBOX=here`): through the proxy, with none of the server's secrets, and with no
  walls around the files or the network (deploy/README.md). Choosing the strongest sandbox the machine has is
  getlatentic/planning#653.
- It finds no engines or model keys by itself: a person names their keys as a shared server's operator does. Finding the ones
  already on the machine is getlatentic/planning#656.
