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
| `local.json`: the ports, the secret, the PATH | in that folder, readable by this person alone. The secret is the server's password and the key its sign-in links are made with. The PATH is the one the person's shell had at install, so the service finds the engines they install |
| Model keys | the computer's keychain: the macOS Keychain, the Windows Credential Manager, or the Secret Service on Linux. Where none can be used, or with `HAAS_KEYCHAIN=off`, `registry/keys` in the data folder, readable by this person alone |
| The models | `registry/` in the data folder |
| The service | macOS a launch agent, `~/Library/LaunchAgents/com.latentic.haas.plist`. Linux a systemd user unit, `~/.config/systemd/user/haas.service`. Windows a task, `Latentic haas`, run at logon |
| The log | `haas.log` in the data folder on macOS and Windows; the journal on Linux |

It listens on `127.0.0.1`, on port 7140 when that is free the first time, and refuses to start on any other address. Its proxy
listens on `127.0.0.1` too, on 7141 when that is free.

## Engines

haas looks for each engine's program on the PATH, each time a page asks, so an engine installed while haas runs is offered at
once. The chat page lists the ones that are missing and the command that installs each, such as
`npm install -g @anthropic-ai/claude-code`. Each job gives its engine a home of its own, with the engine's configuration in it and the
conversation's memory kept with the conversation, so it never reads the person's own configuration of the same program.

## Models and keys

Until there is a model, the chat page asks for one: the provider (Anthropic, OpenAI, DeepSeek, OpenRouter, Ollama on the same
computer, or another address), whether it answers the way OpenAI's API does or Anthropic's, the key, and the models. **List my
models** asks the provider which models the key can use. Every engine is offered every model; the proxy translates Claude Code's
and Codex's calls for a provider that answers Chat Completions alone. More providers are added on the console's Models page.

A key the person's shell already has is offered and not taken. The install names the ones it sees, by name alone, and the
chat page names them too. `haas local use NAME` adds one: it reads the key from the shell it is run in, signs in to haas with a
link of its own, and adds the provider and the model the person picks.

```bash
haas local use ANTHROPIC_API_KEY                     # pick a model from the provider's list
haas local use DEEPSEEK_API_KEY deepseek-chat        # or name it
```

It knows `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `DEEPSEEK_API_KEY` and `OPENROUTER_API_KEY`.

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
haas local use NAME    # add the model key in the shell's NAME
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
- Hermes is started through `sh`, which Windows does not have.
