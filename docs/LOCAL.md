# gusto on your own computer

gusto runs as a service of one person, on the loopback, in a folder of their own. It is the same server as a shared one.

## Install

```bash
# macOS and Linux
curl -fsSL https://github.com/getlatentic/gusto/releases/latest/download/install.sh | sh
# Windows, in PowerShell (in the Command Prompt, run it through: powershell -ExecutionPolicy ByPass -c "...")
irm https://github.com/getlatentic/gusto/releases/latest/download/install.ps1 | iex
```

The script installs `gusto` and `gusto-helper` ([DESKTOP.md](DESKTOP.md#installing-the-helper)). It runs `gusto local install`, which makes gusto a service that starts at login, starts it, and opens it in the browser, signed in. `GUSTO_NO_START=1` installs without starting. While the releases are private, the script needs the GitHub command line, signed in. The desktop app uses this gusto when no organisation is set ([desktop/README.md](../desktop/README.md)).

## Commands

```bash
gusto open              # open it in the browser, signed in (a one-use link, valid five minutes)
gusto local install     # make it a service that starts at login, start it, open it
gusto local run         # what the service runs: the server, in the foreground
gusto local stop        # stop it until the next login
gusto local uninstall   # remove the service, and say where the data folder stays
gusto local use NAME    # add the model key in the shell's NAME
```

gusto listens on `127.0.0.1` only (port 7140 when free, its proxy on 7141) and refuses any other address.

## What it keeps

| What | Where |
| --- | --- |
| Data folder | macOS `~/Library/Application Support/gusto/data`; Linux `$XDG_DATA_HOME/gusto` or `~/.local/share/gusto`; Windows `%LOCALAPPDATA%\gusto\data`. `GUSTO_LOCAL_HOME` names another |
| `local.json` (ports, secret, PATH) | the data folder, readable by you alone |
| Model keys | the keychain (macOS Keychain, Windows Credential Manager, Secret Service). Without one, or with `GUSTO_KEYCHAIN=off`: `registry/keys` in the data folder |
| The service | macOS `~/Library/LaunchAgents/com.latentic.gusto.plist`; Linux `~/.config/systemd/user/gusto.service`; Windows the task `Latentic gusto` |
| The log | `gusto.log` in the data folder (macOS, Windows); the journal (Linux) |

## The wall between an agent and your files

gusto runs jobs behind the strongest wall this computer offers.

| Wall | Needs | Effect |
| --- | --- | --- |
| Virtual machine | Linux with `/dev/kvm`, or a Mac with hardware virtualization; `openshell`, a gateway that answers `openshell sandbox list`, `GUSTO_OPENSHELL_IMAGE` | Each job sees only its own folder |
| Container | Docker or Podman, the runner image on this computer (`GUSTO_IMAGE`, `gusto:dev` by default; gusto does not pull it), a runtime that accepts a unix socket | The same folder. The kernel is shared |
| Process sandbox | macOS `sandbox-exec` (Seatbelt), or Linux 6.7 or later (Landlock) | Writes only the conversation folder, the job's home and state, and temporary files. Cannot read the rest of your home (`~/.ssh`, `~/.aws`, browser profiles, gusto's secret). Reaches only gusto's proxy |
| Nothing | | The agent runs as you |

The chat page names the wall under the message box, and **Why not stronger?** says what each stronger wall lacked. `gusto doctor` prints the same. With nothing, you confirm once on the chat page. The confirmation is the file `no-sandbox-confirmed` in the data folder and the change record entry `sandbox.confirmed`. `GUSTO_SANDBOX` overrides the choice: `openshell`, `here` (nothing), or anything else for a container.

Docker on macOS and Windows is not a wall: its virtual machine cannot share the unix socket a job reaches the proxy through. Windows, and Linux without a VM, a container or Landlock version 4, run jobs with nothing.

Get the virtual machine wall on an Apple Silicon Mac or Linux with `/dev/kvm` with `gusto local wall install` (`--yes` agrees without asking, `--image FILE` names a root file system archive). It checks the computer, downloads OpenShell (about 72 MB on a Mac) and the Linux image the agents run in (about 1.1 GB), checks the checksums, and keeps a gateway named `gusto` running. It needs no password and no Homebrew. `gusto local wall uninstall` removes it all.

The process sandbox blocks downloads (`npm install`, `git clone`, `curl`) and a check's own server on the loopback. `GUSTO_JOB_NETWORK=open` opens the network and keeps the rest. On Linux it does not limit UDP, unix sockets, or reading your other processes' environment through `/proc`. A kernel before 6.7 has no network rules, so gusto does not offer it there unless `GUSTO_JOB_NETWORK=open`.

## Models and keys

gusto finds engines on the PATH. The chat page lists missing ones with the install command, such as `npm install -g @anthropic-ai/claude-code`. Until there is a model, the chat page asks for a provider (Anthropic, OpenAI, DeepSeek, OpenRouter, Ollama, or another address), the API shape, the key and the models. **List my models** asks the provider. Every engine is offered every model. Add providers on the console's Models page.

A key your shell already has is offered, not taken. `gusto local use` adds it. It knows `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `DEEPSEEK_API_KEY` and `OPENROUTER_API_KEY`:

```bash
gusto local use ANTHROPIC_API_KEY                     # pick a model from the provider's list
gusto local use DEEPSEEK_API_KEY deepseek-chat        # or name it
```

To test an install without a release, build a release-shaped folder with `deploy/install/test-release.sh`, serve it, and point `GUSTO_RELEASE_URL` at it. The build is unsigned, so a machine with cosign refuses it. Hermes does not start on Windows (getlatentic/planning#663). More is in the [roadmap](ROADMAP.md).
