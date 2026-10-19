# Quickstart: gusto on your computer in five minutes

You need a computer (macOS, Linux or Windows), and a key for a model you can pay for (DeepSeek, OpenAI, Anthropic, OpenRouter) or Ollama
running on the same computer.

## 1. Install

macOS and Linux:

```bash
curl -fsSL https://github.com/getlatentic/gusto/releases/download/v0.1.0-rc.2/install.sh | sh -s -- v0.1.0-rc.2
```

Windows, in PowerShell:

```powershell
$env:GUSTO_TAG = "v0.1.0-rc.2"; irm https://github.com/getlatentic/gusto/releases/download/v0.1.0-rc.2/install.ps1 | iex
```

It puts gusto in `~/.local/bin` (Windows: `%LOCALAPPDATA%\gusto`), starts it as a service that runs at login, and opens it in your browser, signed in.
If the browser does not open: `gusto open`.

## 2. Add a model

The first page asks for one. Pick your provider, paste the key, press **List my models**, choose one. The key goes in your computer's keychain.

Or from a terminal, if your shell already has the key:

```bash
gusto local use DEEPSEEK_API_KEY       # also ANTHROPIC_API_KEY, OPENAI_API_KEY, OPENROUTER_API_KEY
```

## 3. Install an agent

gusto runs agents it finds on your PATH. The page lists the ones it is missing, with the command for each:

```bash
npm install -g opencode-ai            # or @agentclientprotocol/claude-agent-acp, @agentclientprotocol/codex-acp
```

## 4. Ask it something

Choose the agent and the model under the message box, and ask: `What is 17 times 23? Answer with just the number.` The answer arrives with how long
it took and what it cost. The page says under the box which sandbox the task ran in.

## 5. Check the record

Every task leaves a record. Open the task from the page, or from a terminal:

```bash
gusto verify "<data folder>/jobs/<task id>"
# 7 files match, and the task had no check to run again
gusto show "<data folder>/jobs/<task id>"
```

The data folder is `~/Library/Application Support/gusto/data` on macOS, `~/.local/share/gusto` on Linux and `%LOCALAPPDATA%\gusto\data` on
Windows. A record made without a signing key says `unsigned`; [KEYS.md](KEYS.md) says how to sign one.

## Next

- A stronger sandbox for each task, on Apple Silicon or Linux with KVM: `gusto local sandbox install` ([LOCAL.md](LOCAL.md)).
- A server of your own, with HTTPS: [deploy/README.md](../deploy/README.md).
- Stop it: `gusto local stop`. Remove it: `gusto local uninstall`.
