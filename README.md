# haas

haas runs the coding agents you already use (Claude Code, Codex, OpenCode, pi, Hermes, DeepSeek Agent) in a sandbox, on the model you pick, within a budget. It keeps your keys out of the agent and signs a record of every job, so you can see what each one did and what it cost. Run it on your computer, your server, or your company's cloud.

## Where you can run it

| You have | Run it like this |
| --- | --- |
| **A local machine** (laptop or desktop; macOS, Linux, Windows) | Install it with one command. It starts at login and opens in your browser. |
| **A homelab** (a spare PC or mini server at home) | `sudo server.sh --no-tls` on the box; use it from any computer on your network. |
| **A VPS** (a cloud machine you rent) | `sudo server.sh --domain agents.example.com`: HTTPS, one password, ready in minutes. |
| **A large enterprise** (a team, your own cloud) | Compose or Helm, single sign-on and roles, spend caps per team, a signed record per job. See [docs/PILOT.md](docs/PILOT.md). |

## Try it on your computer

```bash
curl -fsSL https://github.com/getlatentic/haas/releases/download/v0.1.0-rc.2/install.sh | sh -s -- v0.1.0-rc.2
```

It installs haas, starts it, and opens it in your browser, signed in. Add a model key on the first page, pick an agent, and ask it something.
haas is a pre-release: it works, and it changes often.

## What you get

- **A sandbox for each job.** The strongest your machine has: a virtual machine, a container, or the system's own sandbox.
- **Your keys stay out of the agent.** A proxy holds them, counts the tokens, and stops a job that passes its cap.
- **A record you can check.** Each job leaves a signed list of what it changed. `haas verify` re-checks it without trusting the machine that made it.

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
