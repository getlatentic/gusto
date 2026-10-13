# Gusto

**Run AI coding agents safely, and prove what they did.**

Gusto runs the agents you already use (Claude Code, Codex, OpenCode, pi, Hermes, DeepSeek Agent) inside a sandbox, on the model you pick,
within a budget. Every job leaves a signed record that anybody can check without trusting your server.

It runs on your own computer, on a server of your own, or for a team.

- **Safe by default.** Each job gets the strongest wall your computer has: a virtual machine, a container, or the system's own sandbox.
- **Your keys stay out of the agent.** A proxy holds them, counts the tokens, and stops a job that passes its cap.
- **Provable.** `haas verify` re-checks a job's record and re-runs its test. It needs nothing from the machine that made it.

## Try it

```bash
curl -fsSL https://github.com/getlatentic/haas/releases/download/v0.1.0-rc.2/install.sh | sh -s -- v0.1.0-rc.2
```

That installs haas, starts it, and opens it in your browser, signed in. Add a model key on the first page, pick an agent, and ask it something.
macOS, Linux and Windows. Gusto is a pre-release: it works and it changes often.

## Run it for a team

```bash
sudo server.sh --domain agents.example.com      # on a Linux machine of your own, with HTTPS
```

Single sign-on, roles, spend caps and a pilot checklist are in [docs/PILOT.md](docs/PILOT.md). Compose, Helm and Ansible are in
[deploy/README.md](deploy/README.md).

## How it works

```
 you / your tools ──▶ haas server ──▶ a sandbox ──▶ the agent ──▶ model proxy ──▶ your model
 (browser, app, API)   sign-in, caps    one per job   edits files    holds the key
                                │
                                └──▶ signed record of the job  ──▶  haas verify
```

## Read more

| | |
| --- | --- |
| [docs/LOCAL.md](docs/LOCAL.md) | on your own computer: models, agents, the sandbox |
| [docs/PROTOCOL.md](docs/PROTOCOL.md) | the calls any client makes to a server |
| [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) | what it trusts, and what it does not |
| [docs/REFERENCE.md](docs/REFERENCE.md) | everything else, in long form |
| [CONTRIBUTING.md](CONTRIBUTING.md) | build it, test it, send a change |

Made by Latentic. Apache License 2.0 ([LICENSE](LICENSE), [NOTICE](NOTICE)).
