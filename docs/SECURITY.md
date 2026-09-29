# Security review

The pages, the API, the model proxy and the runner were reviewed against the
[threat model](THREAT_MODEL.md) before a customer's security team looks. This
records what was checked, what was found, and what was done.

## How it was done

- Two reviews by reading the code end to end, one of the web tier and one of the
  proxy, the runner and the box, each assuming the caller or the agent is hostile.
  A claim was kept only when it was traced through the code.
- Every finding that was fixed was reproduced or tested first where it could be,
  and has a test that fails without the fix.
- `govulncheck` over the Go code and its standard library, and `npm audit` over
  the pinned npm packages the runner image carries.

Not reviewed: the identity provider, nginx and TLS on the host, the ansible
vault, the deploy key's reach, third-party harness internals, and what a model
does with a hostile prompt.

## Findings

| # | Severity | Finding | Status |
| --- | --- | --- | --- |
| 1 | High | A repository address that began with a dash was read by git as an option. `--upload-pack=<command>` runs the command on the host, outside any box, when a person or an API caller names it as the repository | Fixed. Addresses are held to https and ssh forms; a dash is refused; `git clone` gets `--`; git may not use `ext::`, `git://` or `http://` |
| 2 | High | With no GitHub App, a repository that was a path on the server's disk was cloned, so any runner could name another person's workspace, or any directory the server can read, and read it back through the file viewer | Fixed. A path is refused unless it is under a directory in `HAAS_LOCAL_REPOS`, which is empty by default |
| 3 | High | The runner is root in the box and staged the agent's workspace with the agent's own git configuration. A filter or an fsmonitor the agent set ran as root at the step that writes the record. Reproduced | Fixed. The runner stages the workspace in a repository of its own with no configuration from the workspace, and no longer trusts the workspace's ownership |
| 4 | High | The Go 1.26.1 standard library on the box carried 19 vulnerabilities this server calls, in `net/http`, `crypto/tls`, `html/template` and others | Fixed. Go 1.26.8, with the tarball's checksum pinned, in the playbook and `go.mod`. `govulncheck` reports none |
| 5 | Medium | A link in a job's `out/` was followed by the pages that show a record, and by the hash that a verifier takes. On a runner with no separate agent user the harness can plant one | Fixed. A record's files are read only when they are files; verify names a link. Not reachable with the agent user in place, so it is depth |
| 6 | Medium | No limit on wrong shared passwords or bearer credentials | Fixed. Ten from one address in ten minutes turns it away |
| 7 | Medium | Nothing kept a page from being framed, and no policy on what a page could run | Fixed. A content security policy with per-response nonces, `X-Frame-Options`, `nosniff`, a referrer policy, HSTS over TLS, and no caching of what is signed in |
| 8 | Medium | A container had the runtime's default capabilities and could gain privilege through a setuid program | Fixed. All capabilities dropped but five, and no-new-privileges. Run in the real image |
| 9 | Medium | The proxy checked an allowed name, then let the dialer resolve it again, and a name that resolved to loopback or a cloud's metadata address was connected | Fixed. A name is resolved once, and an inward answer is refused |
| 10 | Low | No limit on the size of a request | Fixed. 4 MB |
| 11 | Low | Signing out only cleared the cookie, so a copy stayed valid for its twelve hours | Fixed. The server ends the session |
| 12 | Low | Google sign-in had state but no PKCE and no nonce | Fixed. Both, tested against a provider that checks them |
| 13 | Low | The model's redirect would have been followed with the credential's headers | Fixed. A redirect is not followed |
| 14 | Info | A spending cap is checked before a call and counted after, so calls in flight when it is reached can pass it | Accepted. The excess is bounded by the calls in flight, and is documented |
| 15 | Info | `haas ship` clones the workspace with `--shared` | Accepted. Needs git 2.38.1 or later, which refuses a source whose `objects/` holds links. The playbook's box has it |
| 16 | Info | Three of the six harness packages in the runner image are not pinned to a version | Open. A pin for `opencode-ai`, `@anthropic-ai/claude-code` and `@earendil-works/pi-coding-agent` means measuring each. The image is built by digest, so a build is still reproducible |
| 17 | Info | On macOS a container runtime's bind mount ignores permissions, so the harness can write `out/` | Documented. Development only |

## Checked and sound

- Every handler that takes an id answers somebody else's object as a missing one:
  jobs, agents, threads, comparisons, files, bundles, the stream, the file viewer.
- The folder viewer resolves links and stays inside the workspace. A bundle stores
  links as links.
- Markdown from a model has raw HTML off, and `javascript:` and `data:` links are
  stripped. Evidence files are plain text with `nosniff`.
- The session cookie is signed, HttpOnly, SameSite=Lax, Secure over TLS, and its
  role cannot be changed without the key. Redirects go to fixed paths.
- An API token is 32 random bytes, kept as a hash, compared in constant time.
- Logs name a credential's kind and never its value, and never a query string.
- The real credential never enters the box, and the GitHub token is minted for one
  repository, handed to git by environment, and revoked afterwards.
- The gate runs without a shell and as the agent user.

## Reporting

Report a vulnerability to the operator of the deployment. A change to anything
listed above should keep its test: `internal/web/security_test.go`,
`internal/web/oidcflow_test.go`, `internal/repoaccess/accept_test.go`,
`internal/runner/workspace_test.go`, `internal/proxy/dial_test.go` and
`internal/sandbox/sandbox_test.go`.
