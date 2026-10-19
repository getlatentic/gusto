# Security review

The pages, the API, the model proxy and the runner were reviewed against the [threat model](THREAT_MODEL.md). This page lists what was checked, found and done.

## How it was done

- Two reviews read the code end to end, one of the web tier and one of the proxy, runner and box. Each assumed the caller or the agent is hostile. A claim was kept only when traced through the code.
- Each fixed finding was reproduced or tested first where possible, and has a test that fails without the fix.
- `govulncheck` ran over the Go code and standard library, and `npm audit` over the pinned npm packages in the runner image.

Not reviewed: the identity provider, nginx and TLS on the host, the ansible vault, the deploy key's reach, third-party agent internals, and what a model does with a hostile prompt. A second review and an outside test are getlatentic/planning#372.

## Findings

| # | Severity | Finding | Status |
| --- | --- | --- | --- |
| 1 | High | A repository address starting with a dash was read by git as an option. `--upload-pack=<command>` ran on the host, outside any box | Fixed. Only https and ssh forms; a dash is refused; `git clone` gets `--`; `ext::`, `git://` and `http://` are off |
| 2 | High | With no GitHub App, a repository that was a path on the server's disk was cloned. Any runner could read another person's workspace through the file viewer | Fixed. A path must be under a directory in `GUSTO_LOCAL_REPOS`, empty by default |
| 3 | High | The runner is root in the box and staged the workspace with the agent's git configuration. A filter or fsmonitor ran as root. Reproduced | Fixed. The runner stages in a repository of its own, with no workspace configuration and no trust in the workspace's ownership |
| 4 | High | Go 1.26.1 on the box carried 19 vulnerabilities this server calls | Fixed. Go 1.26.8, tarball checksum pinned, in the playbook and `go.mod`. `govulncheck` reports none |
| 5 | Medium | Pages and the verifier's hash followed a link in a task's `out/`. A runner with no separate agent user lets the agent plant one | Fixed. Record files are read only when they are files; verify names a link. Depth only, since the agent user blocks it |
| 6 | Medium | No limit on wrong shared passwords or bearer credentials | Fixed. Ten from one address in ten minutes turns it away |
| 7 | Medium | Pages could be framed and had no script policy | Fixed. CSP with per-response nonces, `X-Frame-Options`, `nosniff`, referrer policy, HSTS over TLS, no caching when signed in |
| 8 | Medium | A container had the default capabilities and could gain privilege through a setuid program | Fixed. All capabilities dropped but five, and no-new-privileges. Run in the real image |
| 9 | Medium | The proxy checked an allowed name, then the dialer resolved it again, so a name could reach loopback or a cloud metadata address | Fixed. A name resolves once, and an inward answer is refused |
| 10 | Low | No limit on request size | Fixed. 4 MB |
| 11 | Low | Signing out only cleared the cookie, so a copy stayed valid for twelve hours | Fixed. The server ends the session |
| 12 | Low | Google sign-in had state but no PKCE and no nonce | Fixed. Both, tested against a provider that checks them |
| 13 | Low | The model's redirect would be followed with the credential's headers | Fixed. A redirect is not followed |
| 14 | Info | A spending cap is checked before a call and counted after, so calls in flight can pass it | Accepted. The excess is bounded by the calls in flight. getlatentic/planning#369 |
| 15 | Info | `gusto pr open` clones the workspace with `--shared` | Accepted. Needs git 2.38.1 or later, which the playbook's box has. `gusto doctor` should check: getlatentic/planning#370 |
| 16 | Info | `opencode-ai`, `@anthropic-ai/claude-code` and `@earendil-works/pi-coding-agent`, three of six agent packages in the runner image, are unpinned | Open. The image is built by digest, so a build is reproducible. getlatentic/planning#361 |
| 17 | Info | On macOS a container runtime's bind mount ignores permissions, so the agent can write `out/` | Documented. Development only. `gusto doctor` should detect it: getlatentic/planning#370 |

## Checked and sound

- Every handler that takes an id answers somebody else's object as missing: tasks, agents, threads, comparisons, files, bundles, the stream, the file viewer.
- The folder viewer stays inside the workspace. A bundle stores links as links.
- Model Markdown has raw HTML off and `javascript:` and `data:` links stripped. Record files are plain text with `nosniff`.
- The session cookie is signed, HttpOnly, SameSite=Lax and Secure over TLS. Its role cannot change without the key.
- An API token is 32 random bytes, stored as a hash, compared in constant time.
- Logs name a credential's kind, never its value, and never a query string.
- The real credential never enters the box. The GitHub token is minted for one repository, passed to git by environment, and revoked afterwards.
- The check runs without a shell, as the agent user.

## How a task is isolated

A task runs in a container or in an OpenShell sandbox, which is a microVM. `deploy/smoke-isolation.sh` runs a task whose agent probes its own box. `deploy/aws/openshell/box-probe.sh` runs the same probe in a sandbox. Both ran on 2026-10-07 against an earlier pre-release, since withdrawn. The container probe runs on every release, in the Compose install and the chart ([PILOT.md](PILOT.md)).

| Probe result | Container | OpenShell sandbox |
| --- | --- | --- |
| Kernel | the host's (`6.17.0-1022-azure` on a GitHub runner) | its own: `6.12.76`, on a host running `7.0.0-1014-aws` |
| Outbound | 1.1.1.1:443, 8.8.8.8:53, 169.254.169.254:80, the host bridge on 22 and 8040, and `host.docker.internal` all closed | the same six closed |
| Network | `--network=none`: loopback and the model proxy's socket | a policy that denies all but the model proxy |
| Names | none resolve | resolve inside the sandbox to invented addresses |
| Agent | user 1000, `CapEff` 0, `no-new-privileges`, seccomp filter | the same, and `CapBnd` 0 |
| `CapBnd` | `cb`: chown, file-mode override, file ownership, setuid, setgid, kept for the runner | none |
| Runtime socket, `/var/lib/gusto` | not in the box | not in the box |
| Limits | `--cpus`, `--memory`, `--pids-limit=512` | `GUSTO_OPENSHELL_CPU`, `GUSTO_OPENSHELL_MEMORY` |
| Host needs | Docker or Podman | KVM (nested virtualization on a cloud machine). The server refuses `GUSTO_SANDBOX=openshell` on Linux without `/dev/kvm` |

- A container shares the host kernel, so a kernel flaw reaches every task and the machine. On Compose the server holds the host's runtime socket, which is root on that machine ([deploy/README.md](../deploy/README.md#how-a-task-gets-its-box)). On Kubernetes tasks run in a Docker daemon in a privileged pod. Neither install separates the agent's code from the operator's machine by kernel.
- A sandbox runs on the same machine as the gateway and the server. UDP other than names, and IPv6, are not probed (`docs/CLOUD.md`).
- The probe tries no exploit. It does not test that one task cannot read another's files or exhaust memory. A pass means nothing is open that should not be, not that the box cannot be escaped.
- Use containers when the agents run the team's own code. Use OpenShell on a KVM host when the code is untrusted or the reviewer needs a kernel between agent and machine.

## Reporting

Report a vulnerability to the operator of the deployment. Keep the tests when you change anything above: `internal/web/security_test.go`, `internal/web/oidcflow_test.go`, `internal/repoaccess/accept_test.go`, `internal/runner/workspace_test.go`, `internal/proxy/dial_test.go`, `internal/sandbox/sandbox_test.go`.

## Follow-ups

Backlog issues in getlatentic/planning: #359 tests, vet and `govulncheck` on every pull request. #360 scheduled dependency and image scans. #361 pin the agent packages. #367 trust a named proxy's forwarded address when limiting sign-ins. #369 hold a spending cap under concurrent calls. #370 `gusto doctor` checks bind-mount permissions and git version. #371 read-only root filesystem and tighter system-call profile. #372 review identity provider, TLS, vault and deploy path, and an outside test.
