# gusto

gusto is a tool that lets you run the AI agents you already use, such as Claude Code, Codex, pi, Hermes, OpenCode and more, on your computer, your server, or your company's cloud.

Each task runs in the strongest sandbox your machine has: a virtual machine, a container, or the system's own sandbox. Spending is capped per task and per team, your keys stay out of the agent's reach, and every task leaves an audit trail of what it changed and what it cost, signed so it can be checked.

## Where you can run it

| You have | Run it like this |
| --- | --- |
| **A local machine** (laptop or desktop; macOS, Linux, Windows) | Install it with one command. It starts at login and opens in your browser. |
| **A homelab** (a spare PC or mini server at home) | Run `server.sh --no-tls` on the box (below); use it from any computer on your network. |
| **A VPS** (a cloud machine you rent) | Run `server.sh --domain agents.example.com` (below): HTTPS, one password, ready in minutes. |
| **A large enterprise** (a team, your own cloud) | Compose or Helm, single sign-on and roles, spend caps per team, an audit trail per task, signed. See [docs/PILOT.md](docs/PILOT.md). |

## Try it on your computer

```bash
curl -fsSL https://github.com/getlatentic/gusto/releases/download/v0.1.0-rc.2/install.sh | sh -s -- v0.1.0-rc.2
```

It installs gusto, starts it, and opens it in your browser, signed in. Add a model key on the first page, pick an agent, and ask it something.
gusto is a pre-release: it works, and it changes often.

## Set up a server

On a Linux machine you own (Debian or Ubuntu), this installs Docker if it is missing, starts gusto, and prints the address and a password:

```bash
curl -fsSL https://github.com/getlatentic/gusto/releases/download/v0.1.0-rc.2/server.sh -o server.sh
sudo sh server.sh --no-tls --release v0.1.0-rc.2                      # a trusted network, plain HTTP
sudo sh server.sh --domain agents.example.com --release v0.1.0-rc.2   # a domain that points here, with HTTPS
```

## What you get

- **A sandbox for each task.** The strongest your machine has: a virtual machine, a container, or the system's own sandbox.
- **Your keys stay out of the agent.** A proxy holds them, counts the tokens, and stops a task that passes its cap.
- **A record you can check.** Each task leaves a signed list of what it changed. `gusto verify` re-checks it without trusting the machine that made it.

## Read more

| | |
| --- | --- |
| [docs/QUICKSTART.md](docs/QUICKSTART.md) | five minutes, step by step |
| [docs/LOCAL.md](docs/LOCAL.md) | on your own computer: models, agents, the sandbox |
| [deploy/README.md](deploy/README.md) | a server: Compose, Helm, Ansible |
| [docs/PROTOCOL.md](docs/PROTOCOL.md) | the calls any client makes to a server |
| [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) | what it trusts, and what it does not |
| [docs/ROADMAP.md](docs/ROADMAP.md) | what is next |
| [docs/REFERENCE.md](docs/REFERENCE.md) | everything else, in long form |
| [CONTRIBUTING.md](CONTRIBUTING.md) | build it, test it, send a change |

Made by Latentic. Apache License 2.0 ([LICENSE](LICENSE), [NOTICE](NOTICE)).
