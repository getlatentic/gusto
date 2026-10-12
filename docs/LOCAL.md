# haas on a person's own computer

One person runs haas on their own computer, where the computer is theirs and the agents work on its folders. It is the same
server as a shared one, started for one person, on the loopback, in a folder of their own.

## Install

On macOS and Linux:

```bash
curl -fsSL https://github.com/getlatentic/haas/releases/latest/download/install.sh | sh
```

On Windows, in PowerShell. Windows refuses to run a downloaded script file by default, so the script is read and run in one
step, which it allows:

```powershell
irm https://github.com/getlatentic/haas/releases/latest/download/install.ps1 | iex
```

In the Command Prompt, the same through PowerShell:

```bat
powershell -ExecutionPolicy ByPass -c "irm https://github.com/getlatentic/haas/releases/latest/download/install.ps1 | iex"
```

The script installs `haas` and `haas-helper` (deploy/install/install.sh, [DESKTOP.md](DESKTOP.md#installing-the-helper)) and then
runs `haas local install`, which makes haas a service of this person's that starts at login, starts it, and opens it in the
browser signed in. `HAAS_NO_START=1` installs without starting. While the releases are private, the script needs the GitHub
command line, signed in.

## The desktop app

The desktop app uses haas on this computer when no organisation is set: the one the install script left running, or the `haas` it
carries, started for as long as the app runs. Measured on an M1 Max, with no haas running: 3.5 seconds from opening the app to the
chat page, signed in. Its icon's menu switches between this computer and an organisation ([desktop/README.md](../desktop/README.md)).

## What it keeps

| What | Where |
| --- | --- |
| The data: jobs, conversations, presets, records | macOS `~/Library/Application Support/haas/data`, Linux `$XDG_DATA_HOME/haas` or `~/.local/share/haas`, Windows `%LOCALAPPDATA%\haas\data`. `HAAS_LOCAL_HOME` names another folder |
| `local.json`: the ports, the secret, the PATH | in that folder, readable by this person alone. The secret is the server's password and the key its sign-in links are made with. The PATH is the one the person's shell had at install, so the service finds the engines they install |
| Model keys | the computer's keychain: the macOS Keychain, the Windows Credential Manager, or the Secret Service on Linux. Where none can be used, or with `HAAS_KEYCHAIN=off`, `registry/keys` in the data folder, readable by this person alone |
| The models | `registry/` in the data folder |
| The service | macOS a launch agent, `~/Library/LaunchAgents/com.latentic.haas.plist`. Linux a systemd user unit, `~/.config/systemd/user/haas.service`. Windows a task, `Latentic haas`, run at logon |
| The log | `haas.log` in the data folder on macOS and Windows; the journal on Linux |

It listens on `127.0.0.1`, on port 7140 when that is free the first time, and refuses to start on any other address. Its proxy
listens on `127.0.0.1` too, on 7141 when that is free.

## The wall between an agent and your files

When haas starts it looks for the strongest wall this computer can put between an agent and the person's files, and runs jobs
behind it:

| Wall | What it needs | What it means |
| --- | --- | --- |
| A virtual machine | Linux with `/dev/kvm`, or a Mac with hardware virtualization; the `openshell` command, a gateway that answers to `openshell sandbox list`, and `HAAS_OPENSHELL_IMAGE` | each job sees only the folder it was given |
| A container | Docker or Podman that answers, the runner image on this computer (`HAAS_IMAGE`, `haas:dev` by default; haas does not pull it), and a runtime that can be given a unix socket | the same folder, and the computer's kernel is shared |
| A process sandbox | a Mac: its own `sandbox-exec` (Seatbelt), or Linux 6.7 or later: Landlock, which haas applies itself and needs nothing installed for; each is tried with a real process | the engine and the checks it runs can write only the conversation's folder, the job's own home and state, and temporary files; cannot read the rest of the person's home (`~/.ssh`, `~/.aws`, a browser's profile, haas's own secret) but for the folders the engine is installed in; and reach nothing on the network but haas's proxy. The computer's kernel is shared |
| Nothing | | the agent runs as the person, and can read and change what they can |

The chat page says which, under the box a message is typed in, and **Why not stronger?** says what each stronger wall lacked. `haas
doctor` says it in the same words. With nothing, the person confirms once, on the chat page, before the first agent starts; the
confirmation is the file `no-sandbox-confirmed` in the data folder and is written to the change record as `sandbox.confirmed`. An
agent is pointed only at the folder of its conversation, and with nothing nothing stops it going further.

`HAAS_SANDBOX` decides instead when it is set: `openshell`, `here` (nothing), or anything else for a container.

**Getting the virtual machine wall.** `haas local wall install` sets it up for a Mac with Apple Silicon, or a Linux computer with
`/dev/kvm`, with one yes from the person. It checks the computer first (hardware, the disk tools `mke2fs`, `debugfs` and `e2fsck`,
and, for a release, its root file system), so nothing is downloaded for a computer that cannot use it. It then downloads OpenShell
(`openshell`, `openshell-gateway`, `openshell-driver-vm`, about 72 MB on a Mac), checks each file against a checksum haas carries,
keeps them in the `openshell` folder of haas's data folder, takes the Linux image the agents run in from the release of haas it is (about 1.1 GB; `--image FILE` names one instead), signs the driver on a Mac, makes the gateway's certificates and
configuration, keeps the gateway running (a launch agent on a Mac, a systemd user service on Linux) under the name `haas` so a
gateway of the person's own is left alone, and records what it made. haas uses it the next time it starts. It needs no password and
no Homebrew. `--yes` agrees without asking; `--image FILE` names a root file system archive; `haas local wall uninstall` takes it all
away.

**Docker on a Mac or Windows is not a wall yet.** It runs in a virtual machine that cannot share the unix socket a job reaches haas's
proxy through (`operation not supported`), so haas tests that and goes on to the next wall, and says why. A path over the network
would do it, and exposes the proxy beyond this computer unless it is built with care. Windows is left out of the container wall until
that is done. A Mac therefore runs jobs behind its own sandbox, and Windows and Linux without a VM or a container run them with
nothing until they have their own (a Linux without a Landlock of version 4, which came with Linux 6.7, has none).

**The Linux process sandbox.** haas starts the engine as `haas sandbox-run`, which restricts itself with Landlock and then becomes the engine, so
the restriction holds for everything the engine starts. The job may read the system's folders (`/usr`, `/etc`, `/lib` and the like), the
folders of its PATH, its own folder and its engine's; it may write its workspace, its home, its state, its cache and `/tmp`; and it may
connect to one TCP port, haas's proxy. Its person's home is closed. If the sandbox cannot start, the job does not run. What it does not
limit: UDP, which is how a name is looked up, unix sockets, and reading the environment of the person's other processes through
`/proc` as the same user. A kernel before 6.7 has no network rules, so haas does not offer this wall there unless `HAAS_JOB_NETWORK=open`.

**What the process sandbox costs.** An agent behind it cannot download (`npm install`, `git clone`, `curl`) and a check that connects to
a server of its own on the loopback is refused, since only haas's proxy is reachable. `HAAS_JOB_NETWORK=open` leaves the network open
and keeps the rest. `sandbox-exec` is marked deprecated by Apple, and Claude Code, Codex and Chrome use it; it has no prompt and
nothing to install. The profile is written beside the job (`sandbox.sb` in its folder).

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

- A virtual machine is chosen only on a computer that already has OpenShell, a gateway and an image; shipping them with haas, so a
  laptop gets one with no setup, is getlatentic/planning#655.
- Hermes is started through `sh`, which Windows does not have.
