# Security

gusto runs agents that read files, run commands and call models, so a hole in it matters. Please report one privately.

**Report a vulnerability** with GitHub's private reporting: on this repository, Security, then Report a vulnerability. Do not open a public
issue or pull request for it. Say what you ran, what you saw, and what you think it allows. We read every report and answer it.

**What is supported.** The newest release, and the main branch. gusto is a pre-release, and a fix goes into the next one.

**What is in scope.** The server and its pages, the API, the model proxy, the runner, and the sandbox around a task (virtual machine,
container, Seatbelt, Landlock). What gusto trusts and what it does not is in [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md), and what has
been reviewed, found and fixed is in [docs/SECURITY.md](docs/SECURITY.md). A finding that contradicts either is the most useful kind.

**Out of scope.** The agents themselves (Claude Code, Codex and the others), your identity provider, and a computer you gave an agent full
access to on purpose (`GUSTO_SANDBOX=here`).
