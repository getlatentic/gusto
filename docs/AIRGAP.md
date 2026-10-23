# Air-gapped install

An air-gapped install has no route to the internet from the build machine, the deploy target, the server or a task's container. It can reach services inside the building. This page lists every place gusto reaches outside and what replaces it.

In the tables, "config" means you change a setting or run a mirror. "Code" means this repository has to change first.

## What to provide inside the building

- An OCI registry, or images loaded by hand (`podman load`).
- A Debian mirror, an npm mirror and the vendored Go modules.
- A git host for this repository and for the tasks' repositories.
- An OpenAI-compatible model server with weights you choose. Codex needs the responses API. gusto never ships a model or weights.
- An OIDC provider of your own, or none and the shared password.
- A certificate from your own CA. gusto does not support an ACME server inside.

Pin and mirror before the first build:

| Item | What |
| --- | --- |
| Base images | `docker.io/library/golang:1.26` and `docker.io/library/node:24-slim`, by digest |
| Go | 1.26.0 or later. The playbook uses `go1.26.1.linux-amd64.tar.gz` |
| Go modules | `github.com/coreos/go-oidc/v3 v3.21.0`, `github.com/go-jose/go-jose/v4 v4.1.4`, `golang.org/x/oauth2 v0.37.0` |
| Debian packages | image: `git`, `ca-certificates`. Host: `git`, `curl`, `acl`, `podman`, `uidmap`, `slirp4netns`, `python3-pymysql`, plus `certbot` if you keep it. The playbook assumes `nginx`, `mysql` and `redis` are present |
| Agent packages | each package and its platform package at one pinned version. Measured on x64: `opencode-ai` 1.18.32 with `opencode-linux-x64` and `opencode-linux-x64-baseline`; `@openai/codex` 0.157.1 with `@openai/codex@0.157.1-linux-x64`; `@anthropic-ai/claude-code` 2.1.283 with `@anthropic-ai/claude-code-linux-x64`. arm64 and musl need their own. Seven tarballs, 958 MiB |
| Ansible collections | `ansible.mysql`, `community.crypto`, as tarballs |

Every package is pinned in `pkg/harness/pins.json` (version and sha512), and `deploy/runner/install.mjs` refuses a tarball that does not match. A mirror has to serve those exact tarballs; `pkg/harness/pin.sh` moves a pin.

## Build the image

| Reaches out | Replace with | Change |
| --- | --- | --- |
| Base images (`Dockerfile:9`, `Dockerfile:20`) | an internal registry as a mirror of docker.io | config |
| `go mod download` from `proxy.golang.org` | `go mod vendor`, then `GOPROXY=off GOFLAGS=-mod=vendor go build ./cmd/gusto`. The vendor tree is 560K and needs `schema/` present | code: the Dockerfile still downloads |
| Go fetching a newer toolchain than `go.mod`'s `go 1.26.0` | `GOTOOLCHAIN=local` with Go 1.26 or later | config |
| `apt-get install git ca-certificates` | an internal Debian mirror, or a base image that has both | code: the Dockerfile cannot name a mirror |
| `npm install -g` of each `HARNESSES` entry from `registry.npmjs.org` | an internal npm registry | code: the Dockerfile cannot name a registry |

## Deploy

| Reaches out | Replace with | Change |
| --- | --- | --- |
| `ansible-galaxy collection install` (`deploy/ansible/requirements.yml`) | the tarballs installed from files, or an internal Galaxy | config |
| apt on the box | an internal Debian or Ubuntu mirror | config |
| `ssh-keyscan github.com` and the clone of `git@github.com:getlatentic/gusto.git` | an internal git host: set `repo_url` | code: the host key lookup names github.com |
| The Go tarball from `go.dev` (`toolchain/tasks/main.yml:7`) | the tarball placed at `{{ app_root }}/go{{ go_version }}.tar.gz` first | code for a variable |
| `go build` on the box downloads modules | the vendored tree | code |
| The image build on the box | the build replacements above, or an image built elsewhere and pushed to your registry | as above |
| `certbot certonly --webroot` to Let's Encrypt | a certificate from your CA. The vhost reads `/etc/letsencrypt/live/<name>/` | code: the vhost only reads that path |

## Run the server

| Reaches out | Replace with | Change |
| --- | --- | --- |
| The identity provider's discovery document at startup, and its token endpoint at each sign-in (Google on greencloud) | an OIDC provider in the building, named by `GUSTO_OIDC_ISSUER`, or the shared password alone | config |
| Model upstreams the proxy adds a credential for (on greencloud, `bedrock-mantle.us-east-1.api.aws` and `api.deepseek.com`) | a model server in the building, as a credential's `address` | config |
| `git clone` of a task's repository, on the host | an internal git host. `repo` is any URL git takes | config |
| `gusto pr open`: `api.github.com` | `GUSTO_GITHUB_API` pointed at GitHub Enterprise Server | config |
| `gusto pr open`: `git push` sends the token only to `https://github.com/`, and the pull request links are built as `https://github.com/...` | links and token header that follow the configured host | code |
| `gusto pr verify`: `git fetch` of the record ref | nothing to change: it fetches from the remote the pull request names | none |
| TLS to any of the above signed by your CA | your CA in the host's trust store | config |

The pages reach nothing. `htmx.min.js`, `tokens.css` and `mark.png` are served from the binary. `gusto pr open` to GitLab or Gitea is not covered: it speaks GitHub's API only.

## Run a task

Agent calls were measured on 2026-09-27: each program ran offline on Linux x86-64 behind a stand-in proxy and model, not in gusto's image through gusto's proxy. Every call fails open, and in a sealed container leaves a `denied` line in `out/proxy.jsonl`.

| Reaches out | Replace with | Change |
| --- | --- | --- |
| opencode: `models.opencode.ai` | `OPENCODE_DISABLE_MODELS_FETCH=1` | code: set it in the image |
| opencode: `registry.npmjs.org` for `@opencode-ai/plugin`, every run | none found. `OPENCODE_DISABLE_AUTOUPDATE`, `OPENCODE_DISABLE_LSP_DOWNLOAD`, `OPENCODE_DISABLE_DEFAULT_PLUGINS` and `OPENCODE_DISABLE_SHARE` did not stop it | none |
| codex: `github.com`, `api.github.com`, `chatgpt.com` | `-c features.plugins=false` | code: set it in the adapter |
| codex: `ab.chatgpt.com` | `-c analytics.enabled=false` | code: set it in the adapter |
| claude: `api.anthropic.com`, five connections a run | `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` | code: set it in the image |
| The task's own dependencies, for example `go test` downloading modules | your mirrors, named in `GUSTO_ALLOWED_HOSTS` | config |
| A container with no credential configured keeps the runtime's default network and is not sealed | a credential whose address is the local model, so every task runs sealed | config; code to make it the default |

## What gusto supports

- Tasks on opencode and codex, sealed.
- `gusto pr verify` against any git host the reviewer can reach.
- `gusto pr open` against GitHub Enterprise Server once the change above is done.

A restricted task on codex is refused even when its model is in the building. The rule checks the adapter's vendor endpoint (`https://api.openai.com`, `adapters/codex.json:25`), not the credential's address (`internal/runner/admit.go:21`, `internal/web/api.go:89`). This blocks the air-gapped customer who uses codex.

claude is not supported. It needs an account or an Anthropic-compatible server, and a change to its adapter.

## Not tested

- Whether tasks pass on a local model. On the one task measured, neither `gpt-oss:20b` nor `llama3.2:3b` finished ([README.md](../README.md)). An air-gapped task runs. It may not pass.
- Whether opencode hangs on a cold machine with no network. The first two runs timed out at 90 seconds, and the next eleven finished.
- Whether pre-installing `@opencode-ai/plugin` into `/root/.config/opencode/node_modules` stops the npm call.
- Whether agents call out later in a run, on a tool call, on compaction or on exit. The stand-in answered one text turn.
- Whether the seal holds on the box's podman.
- Whether npm calls anything beyond the registry during the image build.
- Whether the proxy allowlist (`pkg/proxy/allowlist.go`, `host:port`) works for an internal mirror on plain HTTP on a non-standard port.
- Where your model server fetches weights, and whether it can serve files placed by hand.
