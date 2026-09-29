# What an air-gapped install breaks

An air-gapped install has no route to the internet from any machine it touches:
not the machine that builds the image, not the box it is deployed to, not the
server, and not a job's container. It may reach services inside the building: a
registry mirror, a git host, an identity provider, a model server. This page
names every place haas reaches outside, says what replaces each one, and ends
with what the product commits to. It answers getlatentic/planning#255.

Every claim below points at a file, or at a run described in
[How this was found](#how-this-was-found), or is listed under
[Open questions](#open-questions) with the test that would settle it.

## How this was found

The code and configuration were read for every outbound address and every
program that fetches: the `Dockerfile`, `image/`, `go.mod` and `go.sum`, every
role under `deploy/ansible`, and `internal/harness`, `internal/proxy`,
`internal/web`, `internal/ship`, `internal/review`, `internal/runner`,
`internal/instance`, `internal/sandbox` and `internal/catalog`.

The harness programs cannot be read that way: they are other people's binaries.
So each one was run on 2026-09-27, on a Linux x86-64 machine, in a network
namespace with nothing but loopback (`unshare -rn`), which is what
`--network=none` gives a container. Inside the namespace ran two stand-ins:

- a proxy on `127.0.0.1:8041`, named in `HTTPS_PROXY` and `HTTP_PROXY` the way
  `internal/sandbox/sealed.go:48-56` names haas's, which writes down every host
  it is asked for and answers 403, as haas's proxy does for a host the allowlist
  does not name;
- a model on `127.0.0.1:11434` that answers one streamed turn saying `done`, in
  the chat completions, responses and Anthropic messages shapes.

Each harness ran from a clean environment (`env -i`, a fresh `HOME`, no git
configuration) under `strace -f -e trace=%network,execve`, so a connection that
ignored the proxy would still show. None did: every run's only direct
connections were to loopback. The arguments were the adapters' own, from
`internal/harness/adapters/`, with opencode given `image/harness/opencode/opencode.json`.

| harness | package | version |
| --- | --- | --- |
| opencode | `opencode-ai` | 1.18.32 |
| codex | `@openai/codex` | 0.157.1 |
| claude | `@anthropic-ai/claude-code` | 2.1.283 |

This is not the same as a run in haas's image through haas's proxy. It measures
what each program asks for with no network, which is the question here; the
seal itself on podman is still unmeasured, as the README says.

## Everything that reaches out

"Config" means an operator changes a setting or runs a mirror, and nothing in
this repository changes. "Code" means this repository has to change first,
including the Dockerfile and the playbook.

### Building the image

| # | what reaches out | where | replaced by | change |
| --- | --- | --- | --- | --- |
| B1 | the two base images, `docker.io/library/golang:1.26` and `docker.io/library/node:24-slim` | `Dockerfile:9`, `Dockerfile:20` | an internal registry, as a mirror of docker.io in the runtime's registry configuration, or the images loaded by hand with `podman load` | config; a build argument for the base image is cleaner (C2) |
| B2 | `go mod download`, which fetches three modules from `proxy.golang.org` | `Dockerfile:15`, `go.mod:5-9` | a vendored module tree and `GOFLAGS=-mod=vendor` | code (C1) |
| B3 | Go fetching a newer toolchain when the one installed is older than `go.mod` asks for | `go.mod:3` says `go 1.26.0` | `GOTOOLCHAIN=local` and a Go of at least 1.26 already present | config |
| B4 | `apt-get install git ca-certificates` from Debian's mirrors | `Dockerfile:21-23` | an internal Debian mirror, or a base image that already carries both | code (C2): the Dockerfile has no way to name a mirror |
| B5 | `npm install -g` of each harness in `HARNESSES`, from `registry.npmjs.org` | `Dockerfile:28-37` | an internal npm registry that mirrors the packages and their platform packages | code (C2): the Dockerfile has no way to name a registry |

B3 was measured. On this machine `go version` in the repository printed
`go: downloading go1.26.0 (linux/amd64)`, because the installed Go was 1.24.7.
The Dockerfile's `golang:1.26` and the playbook's `go_version: "1.26.1"` both
satisfy `go.mod`, so neither triggers it today; a build on an older Go does.

B2 was measured both ways. With no module cache and no network,
`go build ./cmd/haas` fails at `internal/web/oidc.go:14`, trying
`proxy.golang.org`. With `go mod vendor` run first, the vendor tree is 560K and
`GOPROXY=off GOFLAGS=-mod=vendor go build ./cmd/haas` succeeds with no network.
`go mod vendor` needs `schema/` present, because `internal/audit`'s tests import
it.

B5 was measured. The three packages resolved to seven tarballs on linux x64:
each package and its platform package, and opencode's also pulled a `baseline`
variant. Together they take 958 MiB on disk. Reading the two install scripts,
`opencode-ai/postinstall.mjs` and `@anthropic-ai/claude-code/install.cjs`, found
no download: the binaries come in the platform packages. Two things a mirror
must carry that a list of names hides:

- codex's platform package is the same package name at another version,
  `@openai/codex@0.157.1-linux-x64`, so mirroring `@openai/codex@0.157.1` alone
  is not enough.
- `HARNESSES` defaults to `opencode=opencode-ai` with no version
  (`Dockerfile:28`), and `deploy/ansible/group_vars/all.yml` lists all three
  with none. A mirror serves what it holds, so an air-gapped build has to pin.

### Deploying

| # | what reaches out | where | replaced by | change |
| --- | --- | --- | --- | --- |
| D1 | `ansible-galaxy collection install` of `ansible.mysql` and `community.crypto`, from Galaxy, on the operator's machine | `deploy/ansible/requirements.yml:3-4`, `deploy/ansible/README.md:9` | the collections downloaded once as tarballs and installed from files, or an internal Galaxy | config |
| D2 | apt on the box: `git`, `curl`, `acl`, `podman`, `uidmap`, `slirp4netns`, `python3-pymysql` | `common/tasks/main.yml:2-10`, `sandbox/tasks/main.yml:5-13`, `store/tasks/main.yml:5-10` | an internal Debian or Ubuntu mirror in the box's sources | config |
| D3 | `ssh-keyscan github.com`, and a clone of `git@github.com:getlatentic/haas.git` | `common/tasks/main.yml:53-57`, `group_vars/all.yml:16` | an internal git host holding a mirror of this repository | `repo_url` is config; the host key lookup names github.com, so code (C6) |
| D4 | the Go tarball from `go.dev` | `toolchain/tasks/main.yml:7` | the tarball placed at `{{ app_root }}/go{{ go_version }}.tar.gz` first, or a variable for the address | code (C6) for a variable |
| D5 | `go build` on the box, which downloads the modules into `GOMODCACHE` | `toolchain/tasks/main.yml:23-31` | the vendored tree from B2 | code (C1) |
| D6 | the image build on the box, which is all of B1 to B5 again | `sandbox/tasks/main.yml:32-43` | the same replacements, or an image built elsewhere and pushed to the internal registry | as B1 to B5 |
| D7 | `certbot certonly --webroot`, an ACME challenge to Let's Encrypt | `nginx/tasks/main.yml:44-52` | a certificate from the building's own CA, or an internal ACME server | code (C6): the vhost only reads `/etc/letsencrypt/live/<name>/` (`nginx/templates/haas.conf.j2:10-11`) |

The smoke role reaches nothing: it makes its repository with `git init` and runs
a fixture harness (`smoke/tasks/main.yml:57-99`).

### The running server

| # | what reaches out | where | replaced by | change |
| --- | --- | --- | --- | --- |
| S1 | the identity provider's discovery document at startup, then its token endpoint at every sign-in; Google on greencloud | `internal/web/oidc.go:52`, `internal/web/oidc.go:142`, `host_vars/greencloud.yml:56` | a provider in the building that speaks OIDC, named by `HAAS_OIDC_ISSUER`; or no provider, and the shared password alone | config |
| S2 | the model upstreams the proxy adds a credential for; on greencloud, `bedrock-mantle.us-east-1.api.aws` and `api.deepseek.com` | `internal/proxy/proxy.go:103`, `host_vars/greencloud.yml:36-45` | a model server in the building, named as a credential's `address` | config |
| S3 | `git clone` of a job's repository, on the host before the container starts | `internal/runner/workspace.go:29`, `internal/instance/instance.go:172` | an internal git host; `repo` is any URL git takes | config |
| S4 | `haas ship`: the GitHub API, `api.github.com` by default | `internal/ship/github.go:34` | `HAAS_GITHUB_API` pointed at a GitHub Enterprise Server | config |
| S5 | `haas ship`: `git push` with the token, which is only ever sent to `https://github.com/` | `internal/ship/git.go:59-65` | the header scoped to the configured host | code (C5) |
| S6 | `haas ship`: the pull request's links and the remote it prints, both written as `https://github.com/...` | `internal/ship/links.go:26-32`, `internal/ship/links.go:46-49`, `internal/ship/plan.go:162-169` | links built from the configured host | code (C5) |
| S7 | `haas check`: `git fetch` of the evidence ref, with the reviewer's own credentials | `internal/review/fetch.go:37` | nothing: it fetches from whatever remote the pull request names | none |
| S8 | TLS to any of the above signed by the building's own CA | the Go binary's default HTTP client, `internal/proxy/proxy.go:44`, `internal/ship/github.go:36` | the CA in the host's trust store | config |

The pages reach nothing. `htmx.min.js`, `tokens.css` and `mark.png` are served
from the binary, and `internal/web/static/PROVENANCE.md` says so for this reason.
A search of `internal/web/templates` and `internal/web/static` found no address
outside the server. The schemas' `$id` fields name `raw.githubusercontent.com`
(`schema/*/v1.json:3`); nothing in the code fetches them.

`haas ship` on a non-GitHub forge (GitLab, Gitea) is not an air-gap question,
and is not covered: `internal/ship/github.go` speaks GitHub's API only.

### Inside a job's container

| # | what reaches out | where | replaced by | change |
| --- | --- | --- | --- | --- |
| J1 | opencode, to `models.opencode.ai`, every run | measured | `OPENCODE_DISABLE_MODELS_FETCH=1` | code (C3), to set it in the image |
| J2 | opencode, to `registry.npmjs.org` for `@opencode-ai/plugin`, every run | measured | nothing tried stops it; it fails and the run goes on | open question 2 |
| J3 | codex, to `github.com` (`git ls-remote https://github.com/openai/plugins.git HEAD`), `api.github.com` and `chatgpt.com` | measured | `-c features.plugins=false` | code (C3) |
| J4 | codex, to `ab.chatgpt.com` | measured | `-c analytics.enabled=false` | code (C3) |
| J5 | claude, to `api.anthropic.com`, five connections a run | measured | `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` | code (C3) |
| J6 | the gate and the harness fetching the job's own dependencies, for example `go test` downloading modules | the job's gate, `internal/runner/gate.go:20` | the building's own mirrors, named in `HAAS_ALLOWED_HOSTS` | config |
| J7 | a container with no credential configured, which keeps the runtime's default network and is not sealed | `cmd/haas/wayout.go:98-99`, `internal/sandbox/sandbox.go:48-70` | a credential whose address is the local model, so every job runs sealed | config today; code (C7) to make it the default |

What each harness did, measured as above. A run is one invocation from a clean
`HOME`; "denied" is a request the stand-in proxy refused.

| harness | runs | denied, per run | with the setting | finished |
| --- | --- | --- | --- | --- |
| opencode, as the image configures it | 8 | `models.opencode.ai`, `registry.npmjs.org` | | 7 of 8 |
| opencode, `OPENCODE_DISABLE_MODELS_FETCH=1` and four other `OPENCODE_DISABLE_*` flags | 5 | `registry.npmjs.org` | the models fetch stops | 4 of 5 |
| codex, as the adapter runs it | 2 | `github.com`, `api.github.com`, `chatgpt.com` twice, `ab.chatgpt.com` | | 2 of 2 |
| codex, `-c features.plugins=false -c analytics.enabled=false`, once with two more keys | 3 | none | all five stop | 3 of 3 |
| claude, with `ANTHROPIC_BASE_URL` at the stand-in | 3 | `api.anthropic.com` five times | | 1 of 3 |
| claude, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` | 3 | none | all five stop | 3 of 3 |

Four runs above did not finish, and are counted as runs rather than left out.
The two opencode runs are open question 1. The two claude runs ended with exit 1
because the stand-in answered `/v1/messages?beta=true` with 404: it did not
expect a query string. Once it handled one, every claude run finished. The calls
home were the same in the runs that failed and the runs that finished.

For codex, each key was also tried alone. `features.plugins=false` removed
`github.com`, `api.github.com` and both `chatgpt.com` calls, and
`analytics.enabled=false` removed `ab.chatgpt.com`. `check_for_update_on_startup=false`
and `feedback.enabled=false` changed nothing in `codex exec`.

For opencode, the four other flags were `OPENCODE_DISABLE_AUTOUPDATE`,
`OPENCODE_DISABLE_LSP_DOWNLOAD`, `OPENCODE_DISABLE_DEFAULT_PLUGINS` and
`OPENCODE_DISABLE_SHARE`, found in the binary's strings. None of them removed
the npm call. opencode's log records it as
`background dependency install failed ... 403 Forbidden - GET https://registry.npmjs.org/@opencode-ai%2fplugin`
and the run carries on.

Every one of these calls failed open: the harness went on and answered. In a
sealed haas container each would leave a `denied` line in `out/proxy.jsonl`
(`README.md`, "Nothing else leaves"), which is noise in the record and nothing
worse. Silencing them matters for the record, and for a sealed run that is
**not** air-gapped, where an allowlist entry for the building's mirrors must not
also carry a harness's telemetry.

### The model

Nothing in this repository fetches model weights or installs a model server.
The only models it names are two local ones in
`image/harness/opencode/opencode.json:4-17`, `llama3.2:3b` and `gpt-oss:20b`
behind Ollama on the host, and two hosted ones on greencloud
(`host_vars/greencloud.yml:36-47`), neither of which is reachable from an air
gap. Where Ollama or any other server fetches weights from is outside the code;
see open question 9.

What the product can say about a local model comes from the README's own table
(`README.md:295-305`): on the one task measured, neither `gpt-oss:20b` nor
`llama3.2:3b` finished. An air-gapped install that runs is not the same as one
whose jobs pass, and the second has no evidence yet (open question 3).

## What must be vendored

Everything below has to be inside the building before the first build.

| item | what exactly | measured or read |
| --- | --- | --- |
| base images | `docker.io/library/golang:1.26`, `docker.io/library/node:24-slim`, pinned by digest | read, `Dockerfile:9`, `Dockerfile:20` |
| Go toolchain | a Go of at least 1.26.0, for the playbook the tarball `go1.26.1.linux-amd64.tar.gz` | read, `go.mod:3`, `group_vars/all.yml:22` |
| Go modules | `github.com/coreos/go-oidc/v3 v3.21.0`, `github.com/go-jose/go-jose/v4 v4.1.4`, `golang.org/x/oauth2 v0.37.0`: a `vendor/` tree of 560K | measured |
| image Debian packages | `git` and `ca-certificates` for `node:24-slim`'s release | read, `Dockerfile:22` |
| host packages | `git`, `curl`, `acl`, `podman`, `uidmap`, `slirp4netns`, `python3-pymysql`; `certbot` if its path is kept; `nginx`, `mysql` and `redis` where the box does not have them | read, the roles; the playbook assumes the last three |
| harness packages | per harness, the package and its platform package at one pinned version: `opencode-ai` with `opencode-linux-x64` and `opencode-linux-x64-baseline`; `@anthropic-ai/claude-code` with `@anthropic-ai/claude-code-linux-x64`; `@openai/codex` with `@openai/codex@<version>-linux-x64`. arm64 and musl hosts need their own | measured on x64 |
| opencode's plugin package | `@opencode-ai/plugin`, only if pre-seeding it stops the run-time fetch | open question 2 |
| harness configuration | `image/harness/`, already in the repository | read |
| Ansible collections | `ansible.mysql`, `community.crypto`, as tarballs | read, `requirements.yml` |
| the pages' assets | `htmx.min.js`, `tokens.css`, `mark.png`: already vendored | read, `internal/web/static/PROVENANCE.md` |
| a model server | one that speaks the OpenAI-compatible API opencode and codex are pointed at; the responses API for codex (`adapters/codex.json`, `wire_api=responses`) | read |
| model weights | the weights that server serves, chosen by the customer | open question 3 |
| the jobs' own dependencies | whatever each repository's gate installs, in the building's own mirrors | read, J6 |

## Open questions

Each one says what would settle it. None is answered by guessing.

1. **Does opencode hang on a cold machine with no network?** The first two
   opencode runs, one with the default configuration and one with the five
   flags, stopped at the 90 second timeout without ever calling the model.
   Their logs end right after the failed npm install. Every one of the eleven
   runs after them finished, including repeats of both configurations. The two
   were the only runs made before opencode had ever run on that machine with a
   network. Settle it by running the image with `--network=none` and the socket
   mounted, on a fresh host, ten times, and counting timeouts.
2. **Can opencode's npm call be stopped?** Pre-install `@opencode-ai/plugin`
   into `/root/.config/opencode/node_modules` at image build, run as in
   [How this was found](#how-this-was-found), and see whether
   `registry.npmjs.org` still appears.
3. **Which open-weights model passes real jobs?** Run the README's task, and a
   harder one, on two or three open-weights models on a local server, and
   publish the table with manifests, the way `README.md:295-305` does.
4. **Do the harnesses call out later in a run?** The stand-in answered one turn
   with text and no tool call. A harness may call home on a tool call, on
   compaction, on a long run or on exit. Repeat the measurement with a stand-in
   that asks for an edit and a shell command, over several turns.
5. **Can claude run air-gapped at all?** Its adapter needs a person's account
   (`adapters/claude.json:15-16`), and no account signs in without a network.
   Pointed at a server by `ANTHROPIC_BASE_URL`, it asked for `/v1/messages`
   under that address, but `HAAS_MODEL_URL` already ends in `/v1`
   (`internal/sandbox/sealed.go:53`), so the adapter cannot reuse `{modelUrl}`.
   It also needs a server that speaks Anthropic's messages API. Settle it by
   running a local server that claims to, through the proxy with
   `auth: anthropic`.
6. **Does the seal hold on the box's podman?** Already open in the README's
   "Nothing else leaves". An air gap does not settle it: a leak into an
   internal network is still a leak.
7. **Does npm itself call anything during the image build beyond the
   registry?** Build the image with only the registry mirror reachable, behind
   a logging proxy, and read the log.
8. **Does the proxy's allowlist, which tunnels HTTPS by host, work for an
   internal mirror that serves plain HTTP on a non-standard port?**
   `internal/proxy/allowlist.go` takes `host:port`; run a job whose gate
   installs from such a mirror.
9. **Where does the chosen model server fetch weights from, and can it serve
   files placed by hand?** For Ollama, run `ollama pull` behind a logging proxy,
   then copy its model directory to a machine with no network and serve from
   it.

## The decision

### What haas commits to

An air-gapped install is supported when the customer runs these inside the
building, and haas needs nothing else:

- an OCI registry, or images loaded by hand;
- a Debian mirror, an npm mirror, and the vendored Go modules;
- a git host for this repository and for the jobs' repositories;
- an OpenAI-compatible model server with the weights they chose;
- an OIDC provider of their own, or none and the shared password;
- a certificate from their own CA.

Jobs run on **opencode** and **codex**, sealed, with the settings above so that
nothing in `out/proxy.jsonl` is a harness calling home. Every job runs sealed:
a deployment with only a local model still names it as a credential, so its
containers have no network but the proxy's socket (J7).

`haas check` works against any git host the reviewer can reach. `haas ship`
works against GitHub Enterprise Server once C5 is done.

### What it does not commit to

- **Model weights or a model server.** haas points at one; it never ships one.
  The model and the credentials are never in a bundle, as `docs/DIRECTION.md`
  already says of bundles.
- **Which model is good enough.** Until open question 3 has a table, the
  product says an air-gapped job runs, not that it passes.
- **claude.** It needs an account or an Anthropic-compatible server, and a
  change to its adapter (open question 5).
- **`haas ship` to a forge that is not GitHub.**
- **An ACME server inside.** A certificate from the customer's CA replaces the
  certbot path.
- **One offline bundle file.** Until a second customer asks, the install is the
  mirrors above, not a tarball of everything.

### The smallest set of changes

Each is a candidate issue. The first four carry the commitment; the rest widen
it.

| | candidate issue title | what it changes |
| --- | --- | --- |
| C1 | build: vendor the Go modules and build with `-mod=vendor` | `vendor/` committed; `Dockerfile:14-15` copies it instead of downloading; the toolchain role builds with `GOFLAGS=-mod=vendor` and `GOTOOLCHAIN=local` |
| C2 | image: build from internal mirrors named by build arguments | `ARG`s for the two base images, an npm registry and a Debian mirror in the `Dockerfile`; nothing changes when they are unset |
| C3 | image: stop the harnesses calling home | `OPENCODE_DISABLE_MODELS_FETCH=1` and `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` in the image's `ENV`; `features.plugins=false` and `analytics.enabled=false` in the codex adapter's `-c` arguments, which is the form measured, or in `image/harness/codex/config.toml`; a test that holds the measured hosts |
| C4 | rules: judge a restricted job by the credential's address, not the adapter's vendor endpoint | `internal/runner/admit.go:21` and `internal/web/api.go:89` check `adapter.EndpointOf()`, which for codex is `https://api.openai.com` (`adapters/codex.json:25`) whatever the credential points at, so a restricted job on codex is refused even when its model is in the building |
| C5 | ship: push to and link a GitHub Enterprise host, not only github.com | the token header in `internal/ship/git.go:64`, the links in `internal/ship/links.go:27-48` and the remote parsing in `internal/ship/plan.go:164` follow the configured host |
| C6 | deploy: install from internal mirrors | the known-hosts entry follows `repo_url` (`common/tasks/main.yml:53-57`); the Go tarball's address is a variable (`toolchain/tasks/main.yml:7`); the vhost takes certificate paths, so certbot is one way to fill them (`nginx/tasks/main.yml:44-52`) |
| C7 | sandbox: seal every job, not only one with a credential | `cmd/haas/wayout.go:98-99` runs a job with no proxy on the runtime's default network; with a local model the proxy is still what records what left |

C4 is the one that surprised. The rule the product already enforces,
"restricted code only talks to a model in the same building", refuses exactly
the air-gapped customer who uses codex.
