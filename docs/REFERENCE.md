# Reference

The [README](../README.md) is the short version. This page has examples, files and settings. [CLI.md](CLI.md) lists every command and flag.

- [Run a task](#run-a-task)
- [Open a pull request and verify it](#open-a-pull-request-and-verify-it)
- [Sign the record](#sign-the-record)
- [Rerun and compare](#rerun-and-compare)
- [Task directory](#task-directory)
- [Audit export](#audit-export)
- [Cost and machine size](#cost-and-machine-size)
- [Adapters](#adapters)
- [Model access and network](#model-access-and-network)
- [Spend caps and report](#spend-caps-and-report)
- [Container and doctor](#container-and-doctor)
- [Environment](#environment)

## Run a task

A task needs a repository, a brief and a check. The check is the command that says pass or fail. It is everything after `--`.

```bash
gusto new /tmp/fix -repo /path/to/repo -brief "make the answer yes" -agent opencode -- go test ./...
gusto inside /tmp/fix     # work in this process
gusto run /tmp/fix        # work in a container
gusto submit /tmp/fix     # send to a server, which runs it
gusto show /tmp/fix       # brief, agent, check, verdict, model, signer
gusto verify /tmp/fix     # prints: 7 files match, and the check still exits 0
```

- `inside` and `run` exit 0 when the check passes.
- `submit` posts the task to `POST /api/jobs` with the task's adapter, so the server needs no copy of your agent.
- `verify` exits 1 and names the file when anything in `out/` changed since the run.
- `gusto serve` runs the pages. A run happens on an instance, which is an agent from the catalog with a folder of its own. `/jobs` records every run.
- `gusto serve` writes one JSON line per request to stderr: method, path, status, bytes, duration, client address and credential kind (`cookie`, `bearer` or `none`). It omits the credential, the query string, `/healthz`, `/readyz` and a `/metrics` scrape that worked. See [RUNBOOK.md](RUNBOOK.md).

Trust and review: [THREAT_MODEL.md](THREAT_MODEL.md), [SECURITY.md](SECURITY.md).

## Open a pull request and verify it

`gusto pr open /tmp/fix` opens a pull request for a passed task. It never merges.

- It re-hashes the record and exits 1 if the check failed or a file changed.
- It pushes `out/diff.patch` on a branch named for the task and its manifest hash.
- It pushes `job.json` and `out/` under `refs/gusto/evidence/<manifest sha256>`, which is not a branch, so the record stays out of the change.
- The pull request names the agent, model and check, links the manifest and transcript, and prints the check command.
- `-dry-run` prints the plan and does nothing. The flags that override what `gusto pr open` reads from the task are in [CLI.md](CLI.md).

A reviewer runs the printed command. It fetches the record ref with the reviewer's own git credentials, re-hashes every file, rebuilds the workspace from `diff.patch` and runs the check. It exits 1 and names what disagrees. `-into <dir>` keeps the unpacked task.

```bash
gusto pr verify <repository> <manifest sha256>
```

## Sign the record

A signature stops whoever stores the record from rewriting it, hashes included. Keep the key where they cannot reach it.

```bash
gusto keygen acme-2026 -dir ~/.gusto     # acme-2026.key (0600) and acme-2026.pub
export GUSTO_SIGNING_KEY=~/.gusto/acme-2026.key   # on the machine that runs tasks
export GUSTO_KEYRING=~/keyring                   # on the machine that verifies
cp ~/.gusto/acme-2026.pub ~/keyring/
```

- The runner signs `out/manifest.json` with Ed25519 and writes `out/manifest.sig`.
- `gusto run` signs on the host after the container is gone, so the key never enters the container.
- With no key, the run works and the record is unsigned.
- To rotate, add the new `.pub` to every keyring and keep the old one. Old signatures still verify. For backup and AWS KMS, see [KEYS.md](KEYS.md).
- `gusto keys` lists which key signed each record.

`verify`, `show`, the task page, the pull request and `audit.json` name the signer from the keyring, never from the manifest's `signer` field.

| Record | They say | `verify` exit |
| --- | --- | --- |
| signed by a keyring key | `<name>` | the check's |
| signed, no `GUSTO_KEYRING` | `signed, not checked: no keyring` | the check's |
| signed by a key not in the keyring | `signed, not checked: key <id> is not in the keyring` | 1 |
| signed, then changed | `signed, and the signature does not hold` | 1 |
| unsigned | `unsigned` | the check's, or 1 with `-signed` |

An unsigned record passes unless `-signed` is given, so records made before signing still verify.

## Rerun and compare

`gusto rerun` runs finished tasks again and compares each new verdict with the old one. Use it to catch an agent upgrade that makes results worse.

```bash
gusto rerun /jobs -here     # ends: 2 rerun: 0 worse, 0 better, 2 same, 0 could not run
gusto compare /tmp/fix claude opencode   # one task on two agents, kept as /tmp/fix-compare-1
gusto compare /tmp/fix-compare-1         # prints it again from the record
```

- `rerun` takes task directories, or a directory of them. It writes `<task>-replay-<n>` beside each, or under `-into`.
- The new task is the old `job.json` and adapter, with `ref` set to the commit the old run started from.
- `rerun` runs in a container, or in this process with `-here`. It skips a task whose record no longer checks.
- `out/replay.json` records the original directory, its manifest hash and the commit. The agent never sees the old verdict.
- `rerun` exits 1 when any verdict is worse, 2 when a task could not be rerun, otherwise 0.
- What a comparison holds constant: [COMPARE.md](COMPARE.md).

## Task directory

`job.json` holds version, brief, repo, check, agent, model, sensitivity, budget and team. `harnesses/` holds a configured agent of your own. `workspace/` is the checkout. `out/` holds the record:

| File in `out/` | Holds |
| --- | --- |
| `transcript.log`, `harness.log`, `stderr.log` | what the agent said, every line it printed, what it complained about |
| `calls.jsonl` | the model calls that reported tokens or cost |
| `proxy.jsonl` | every call that left through the proxy, and its cost |
| `commands.jsonl` | what the agent ran |
| `diff.patch`, `gate.log`, `gates.json` | what changed, the check's output, the verdict |
| `manifest.json` | a hash of each file above, the totals and the signer |
| `manifest.sig` | the signature over `manifest.json`, when the runner has a key |

A token count or cost is `null` when the agent does not report one. Zero would read as free. `job.json`, `manifest.json` and `manifest.sig` have versioned JSON Schemas in [`schema/`](../schema/). [FORMATS.md](FORMATS.md) says when versions change.

## Audit export

`gusto export <dir>` writes one task's audit record to `<dir>.audit`, or to the directory `-to` names. It holds `audit.json`, `job.json`, `state.json` (when the server has one), `requests.jsonl` (with `-requests`) and `out/`.

`audit.json` says who asked, what ran, what changed, which model, what it cost and who approved. It says `not recorded` where the platform keeps nothing. Today that is who asked (everybody shares one password) and who approved (a person merges on the forge).

`gusto export -verify task.audit` checks the manifest and every file hash, derives the summary again and compares it with `audit.json`. It exits 1 and names the file when anything disagrees. It does not re-run the check.

`-policy <file>` sets what an export drops and hides. A dropped file is not carried. A redacted field becomes `"[redacted]"`. A pattern match becomes `[redacted: api-key]`. `audit.json` lists each changed file with its original hash. See [FORMATS.md](FORMATS.md#the-audit-export).

```json
{
  "drop": ["out/transcript.log", "requests.jsonl"],
  "redact": {
    "fields": ["brief", "remote"],
    "patterns": [{"name": "api-key", "match": "sk-[A-Za-z0-9-]{16,}"}]
  }
}
```

## Cost and machine size

One task, three ways: a one-line shell fix, with a shell script as the check.

| Agent and model | Finished | Wall clock | Billed tokens | Cost |
| --- | --- | --- | --- | --- |
| `claude`, Opus 5 | yes | 2m55s | 1,134,184 | $1.3161 |
| `opencode`, `ollama/gpt-oss:20b` | no | 2m01s | 16,669 | $0 |
| `opencode`, `ollama/llama3.2:3b` | no | 1m14s | 30,138 | $0 |

Claude's total is 1,065,920 cache reads across 41 turns, plus 61,508 cache writes. The check refused both local runs. One task is not a benchmark. To measure many pairs, see [MATRIX.md](MATRIX.md).

An `opencode` task peaked at 662.7 MiB in its container, with the model outside it. Plan about 0.7 GiB for each concurrent task, plus the container runtime. Docker Desktop on a Mac costs about 2 GiB. On Linux, `GUSTO_CONTAINER=podman` needs no daemon.

## Adapters

An agent is the program: `claude`, `codex`, `opencode`. An adapter is how to run it. A configured agent is the adapter's directory plus its prompt, model policy, skills and MCP servers. Every run records it by digest. See [TERMS.md](TERMS.md).

`gusto agents` lists them with what each needs: `nothing`, `local`, `account` or `key`. With `nothing`, no model is involved and no endpoint is checked.

## Routines

A routine is a question an agent is asked on a schedule, in a conversation of its own, so the agent remembers from one run to the next. Add one on the agent's page under Routines: a title, what to ask, and either a time of day (`08:00`, in the server's own zone) or a number of hours between runs. Once a minute the server starts every routine whose slot has come as an ordinary turn: through the proxy, under the team's cap, and into the signed record, with the brief `[routine] <title>: <what to ask>`. A routine whose last turn is still answering skips the slot, and the page says so. A routine added after today's time waits for tomorrow's. Routines are kept in `routines.json` in the agent's folder. The person who added a routine is told how each run went the way they are told of any turn of theirs, on the events a tray listens to (`/api/events`). Two more ways are off until set. `GUSTO_NOTIFY_WEBHOOK` is an https address, with `GUSTO_NOTIFY_SECRET`: it is sent `{event, instance, routine, title, job, state, at, link, who}` as JSON when a run ends, with `X-Gusto-Signature: sha256=<HMAC-SHA256 of the body under the secret>`, and never the agent's words. `GUSTO_MAIL` (`outbox`, `smtp://user:password@host:587?from=name@example.com` or `cloudflare://<account id>?from=name@example.com`, with `GUSTO_MAIL_TOKEN` for Cloudflare) mails the person who added the routine, when that person is an address, with the end of what the agent said. `GUSTO_PUBLIC_URL` is where the server's own pages are, for the link. Each channel takes an event once; one that is down is tried again each minute, five times, and the routine's page says why when it gives up. Pause, resume and remove are on the same page; removing a routine keeps its conversation.

An adapter is a JSON file. A directory named by `GUSTO_HARNESSES` overrides a built-in adapter of the same name, or adds one. It holds bare adapters (`name.json`) or configured agents (`name/adapter.json` with its files beside it, reached through `{harnessDir}`).

```json
{
  "name": "opencode",
  "program": "opencode",
  "argv": ["run", "{brief}"],
  "needs": "nothing",
  "stream": "plain"
}
```

- `{brief}`, `{model}` and `{modelUrl}` come from the task.
- A `jsonl` stream adds dotted paths: `text`, `inputTokens`, `outputTokens`, `costUsd`. `totals` says whether the agent reports per call or as a running total.
- An `acp` stream holds the conversation over the Agent Client Protocol, and the paths default to where the protocol says things. `authenticate` names the agent's authentication method, by its id, for one that refuses a session until it is chosen (`codex-acp` lists `api-key`); the credential itself is in `env`. Claude Code and Codex run through their maintained ACP adapters, `@agentclientprotocol/claude-agent-acp` and `@agentclientprotocol/codex-acp`.
- Write a path only after a real run shows where the number sits. An unmeasured stream is read as plain text, and the numbers are left out.
- Cache tokens are counted, because they are billed.

## Model access and network

The container never holds a model credential. It gets `GUSTO_MODEL_URL=http://127.0.0.1:8041/model/v1` and the task's token as its key. The runner relays that port to the proxy's socket. The proxy checks the token, adds the real credential and records the call.

- The token is minted per task and dropped when the task ends. Any authorization the container sends is discarded.
- `out/proxy.jsonl` records what left the box. An agent's own usage report is a claim. This is the count.
- Without `GUSTO_MODEL_UPSTREAM` there is no proxy, and the container gets the model's address directly.
- A container with a proxy runs with `--network=none`. The only way out is the proxy's socket at `/run/gusto/proxy.sock`.
- The proxy refuses every host except the model unless `GUSTO_ALLOWED_HOSTS` names it. Nothing set allows nothing.
- `host` allows ports 80 and 443. `host:port` allows that port. `*.example.com` allows every name under the domain, not the domain itself.
- Every request is a line in `out/proxy.jsonl` with its `host`. A refused one says `denied` and gets a 403. For HTTPS the proxy sees the host, not the content.
- Translation, tools and masking: [GATEWAY.md](GATEWAY.md).

```bash
GUSTO_ALLOWED_HOSTS=registry.npmjs.org,proxy.golang.org,*.githubusercontent.com
```

Not yet measured: that `--network=none` with a mounted socket holds on podman (rootless or not) and on Docker, and whether Docker Desktop on a Mac hands a container a host socket. `GUSTO_JOB_NETWORK=open` keeps the runtime's network and reaches the proxy on `GUSTO_PROXY_ADDR`. That route is not sealed. The allowlist covers only traffic through the proxy.

## Spend caps and report

The proxy counts each call from the provider's `usage` object (`prompt_tokens`, `completion_tokens`), in a plain answer and in the last event of a stream. This path is not yet measured against a real upstream. `GUSTO_SPEND_POLICY` names a file of prices and monthly team caps. The numbers show the format only.

```json
{
  "prices": { "some-model": { "inputPerMillion": 2.5, "outputPerMillion": 10 } },
  "teams": { "platform": { "usdPerMonth": 200 } }
}
```

A field this build does not know stops the server, because a misspelt cap would be no cap. `inputTokens`, `outputTokens` and `usd` in `out/proxy.jsonl` are `null` when the answer has no usage or the model has no price: unmeasured, not free.

- A task's cap is `budget.usd` in `job.json`. Left out is no cap. Zero allows nothing.
- A team's cap is per calendar month in UTC. A task counts against its `team` field, a label the person asking chooses. The cap holds against mistakes, not against a determined spender.
- The proxy checks both before each call. When one is spent it answers `402` with the reason. A call in flight finishes.
- Under a cap, a model with no price is refused. A successful answer with no usage stops the next call.
- The agent is not killed. It loses the model, and the time budget ends it. The manifest records `stoppedByCap`, and `gusto show` names the cap.
- A restarted server rebuilds each team's month from the tasks' `out/proxy.jsonl`.

```bash
gusto spend                   # this month, from GUSTO_JOBS
gusto spend -month 2026-08    # another month
gusto spend -json             # for a script
```

The report groups by team, project (the task's repository) and task. The `spend` page shows the same. `GET /api/spend?month=2026-09` returns the JSON, with the credential `POST /api/jobs` takes: an API token that may read spending, or the shared password when the server has no tokens ([TOKENS.md](TOKENS.md)). A sum that includes unmeasured calls reads `at least $…`, or `unmeasured`. The totals are not reconciled with a provider invoice.

## Container and doctor

```bash
docker build -t gusto:dev .
GUSTO_MODEL_URL=http://localhost:11434/v1 gusto run /tmp/fix
```

`run` mounts the task directory, allows 512 processes and the limits in `GUSTO_JOB_CPUS` and `GUSTO_JOB_MEMORY`, and destroys the container afterwards. On a shared machine, set the limits. `GUSTO_IMAGE` names another image.

`HARNESSES` lists the agents an image carries as `name=package` pairs (adapter name, npm package). The default is `opencode=opencode-ai`. The version of each package is pinned in `pkg/harness/pins.json` with the registry's sha512, and the build refuses a tarball that differs; the image records what it installed in `/etc/gusto/harnesses`, which the record names as the agent's build (`harnessVersion`). `gusto serve` reads the image label `com.getlatentic.gusto.harnesses`, checks the image's PATH for each program, and offers only those it finds. A task that names a missing agent fails before it starts.

```bash
docker build -t gusto:dev \
  --build-arg HARNESSES="opencode=opencode-ai claude=@agentclientprotocol/claude-agent-acp codex=@agentclientprotocol/codex-acp" .
```

- Each agent's configuration is a directory under `pkg/harness/config/`, copied to `/root/.config/<name>/`.
- A bind mount of a path the runtime does not share gives an empty directory. `run` refuses by name. Docker on a Mac shares `/Users`, not `/private/tmp`.
- A model on the host must listen on more than loopback. For Ollama, set `OLLAMA_HOST=0.0.0.0` and restart.

`gusto doctor` checks the machine before a task. It prints one line per check, gives the fix under a failure, and exits 1 when any fails. It checks the container command, the image, the task's `--memory` and `--cpus`, the tasks directory mount, rootless podman's runtime directory, `git`, `GUSTO_JOBS`, `GUSTO_MODEL_URL`, `GUSTO_HARNESSES` and the index of model calls (it opens, and at what schema version). It reports whether `GUSTO_PASSWORD` is set, never its value.

## Environment

| Name | What it does |
| --- | --- |
| `GUSTO_HARNESSES` | a directory of adapter files that override the built-in ones |
| `GUSTO_MODEL_URL` | where a local model answers |
| `GUSTO_IMAGE` | the runner image `gusto run` starts |
| `GUSTO_CONTAINER` | the container command: `docker` by default, `podman` for no daemon |
| `GUSTO_JOB_MEMORY`, `GUSTO_JOB_CPUS` | memory and CPU for one task, `3g` and `1` by default |
| `GUSTO_JOB_CONCURRENCY` | how many tasks run at once |
| `GUSTO_LIVE_CONNECTIONS` | the most WebSockets open at once, 10000 by default. A room's page keeps one open at `/live` to be told what others say and what agents do, and asks every few seconds instead when it has none. A proxy in front must pass the upgrade and not buffer or cut off a quiet connection: the server pings every 30 seconds |
| `GUSTO_LIMIT_PERSON`, `GUSTO_LIMIT_TEAM` | the most one person or team runs at once. A task over its limit waits. 0 or unset is no limit. [LIMITS.md](LIMITS.md) |
| `GUSTO_SANDBOX` | how a task runs: unset is a container, `here` is this process, `openshell` is an OpenShell sandbox. [OPENSHELL.md](OPENSHELL.md) lists the `GUSTO_OPENSHELL*` settings |
| `GUSTO_OPENSHELL_REMOTE` | `1` when the OpenShell gateway is on another machine. Without it, `gusto serve` and `gusto doctor` refuse `GUSTO_SANDBOX=openshell` on Linux with no `/dev/kvm` |
| `GUSTO_LOCAL_REPOS` | server directories a task's repository may be cloned from. Unset, only https and ssh addresses work. [SECURITY.md](SECURITY.md) |
| `GUSTO_MODELS` | the models the pages offer, comma separated. With credentials configured, an agent is offered only models a credential serves. A person can change their agent's model on its page |
| `GUSTO_MODEL_UPSTREAM` | the model's root address, with no version. Setting it turns the proxy on |
| `GUSTO_MODEL_KEY` | the credential the proxy adds. It never reaches a container |
| `GUSTO_MODEL_NAME` | what the record calls that credential |
| `GUSTO_CREDENTIALS` | a directory with one `<name>.json` per credential: address, key, presentation and the API shapes it answers. [GATEWAY.md](GATEWAY.md) |
| `GUSTO_PROXY_SOCKET` | the socket a sealed container reaches the proxy through, `.proxy.sock` in `GUSTO_JOBS` by default |
| `GUSTO_ALLOWED_HOSTS` | hosts besides the model a task may reach, comma separated |
| `GUSTO_JOB_NETWORK` | `open` gives containers the runtime's network and the proxy on `GUSTO_PROXY_ADDR` |
| `GUSTO_PROXY_ADDR` | where the proxy listens when `GUSTO_JOB_NETWORK=open`, `0.0.0.0:8041` by default |
| `GUSTO_SPEND_POLICY` | a file of model prices and team caps. Without it every model is unpriced and no team is capped |
| `GUSTO_REGISTRY` | the directory of an organization's model registry: providers and models its admins add at `/admin/models`, on top of the operator's. Set by gusto cloud. [CLOUD.md](CLOUD.md) |
| `GUSTO_REGISTRY_KEYS`, `GUSTO_REGISTRY_PRIVATE` | `file`, `ssm` or `secretsmanager` lets a key be a reference to one of these. `allow` lets a provider be at a private address or plain http |
| `GUSTO_ORG_SETTINGS` | the file of an organization's admin choices, such as the monthly budget. Set by gusto cloud. [CLOUD.md](CLOUD.md) |
| `GUSTO_SIGNING_KEY` | the private key the runner signs with, from `gusto keygen`, or `awskms://<key>`. It stays on the host. [KEYS.md](KEYS.md) |
| `GUSTO_KEYRING` | a directory of `<name>.pub` keys that `verify`, `show`, `serve`, `pr open` and `export` trust, retired keys included |
| `GUSTO_SIGNER` | who the manifest names. This is the runner's own word |
| `GUSTO_CHANGES` | the file recording platform changes: sign-ins, refusals, role, setup and settings changes. Hash-chained, and signed when a key is set. `gusto changes verify` checks it. [CHANGES.md](CHANGES.md) |
| `GUSTO_RETENTION` | the directory holding `policy.json`, the holds and the last sweep. A task's workspace is removed first, its signed record later. `GUSTO_RETENTION_MINUTES` sets the interval, 60 by default. `gusto retention` lists what a sweep would remove. [RETENTION.md](RETENTION.md) |
| `GUSTO_GITHUB_TOKEN` | the token `gusto pr open` pushes and opens pull requests with. `GUSTO_GITHUB_API` overrides the API address |
| `GUSTO_GITHUB_APP_ID`, `GUSTO_GITHUB_APP_KEY` | a GitHub App that reaches the repositories tasks name. Each task gets a token for one repository, revoked when the work is done. Set both or neither. [REPOS.md](REPOS.md) |
| `GUSTO_GITHUB_URL` | a GitHub Enterprise Server's address, with `GUSTO_GITHUB_API`. github.com when unset |
| `GUSTO_ADDR` | where `gusto serve` listens, `127.0.0.1:8040` by default |
| `GUSTO_JOBS` | the directory `gusto serve` keeps tasks in |
| `GUSTO_DATABASE_URL` | where the indexes are kept: the index of model calls and the index of tasks. Unset, they are `usage.db` and `tasks.db` in the directory that holds `GUSTO_JOBS`. A `postgres://` address keeps both in Postgres, and its `search_path` parameter names the schema, which is how organizations that share a database stay apart. The tasks' `out/proxy.jsonl` and `state.json` stay the record: the indexes are built from them, and `gusto usage reindex` and `gusto tasks reindex` rebuild them. Retention leaves the model calls' rows, which hold no content, and removes a removed task's row. [USAGE.md](USAGE.md), [TASKS.md](TASKS.md) |
| `GUSTO_PASSWORD` | the shared sign-in. `gusto serve` refuses to start without it |
| `GUSTO_OIDC_ISSUER`, `GUSTO_OIDC_CLIENT_ID`, `GUSTO_OIDC_CLIENT_SECRET`, `GUSTO_OIDC_REDIRECT` | sign in with the company's identity provider. Naming an issuer turns it on. It needs the other three, and `GUSTO_RUNNER_GROUPS`, `GUSTO_ADMIN_GROUPS` or `GUSTO_ALLOWED`, or the server refuses to start. [SSO.md](SSO.md) |
| `GUSTO_OIDC_SCOPES`, `GUSTO_OIDC_GROUPS_CLAIM` | scopes to ask for besides `openid` and `email`, and the claim the groups arrive in (`groups` by default) |
| `GUSTO_RUNNER_GROUPS`, `GUSTO_ADMIN_GROUPS` | the provider's groups whose members are runners, and admins. A person in neither is refused |
| `GUSTO_ALLOWED` | addresses, or a domain written `@example.com`, that may sign in. With no groups named it is the whole check |
| `GUSTO_ADMINS` | addresses that are admins whatever their groups say |
| `GUSTO_API_TOKENS` | the file API tokens are kept in. With it the API takes scoped, revocable tokens and refuses the shared password. [TOKENS.md](TOKENS.md) |
| `GUSTO_MIGRATE_IRREVERSIBLE` | `1` lets a start run a data migration that cannot be undone. Without it the server refuses and names the step. [UPGRADE.md](UPGRADE.md) |
| `GUSTO_DEPLOY_RESULT` | a file a deploy writes when it ends, `{"release","ok","at"}`. `/metrics` reports it as `gusto_last_deploy_ok`. [RUNBOOK.md](RUNBOOK.md) |
| `GUSTO_SERVER` | where `gusto submit` sends a task, with its scheme |
| `GUSTO_TOKEN` | the API token `gusto submit` sends, made at `/admin/tokens`. `GUSTO_PASSWORD` is sent when it is unset |
| `GUSTO_MEMBERS`, `GUSTO_INBOX` | `GUSTO_MEMBERS` is the file of people the console shows at `/admin/people`. `GUSTO_INBOX` is where gusto cloud leaves what its admins did to those people |
| `GUSTO_CLOUD_DEMO_MINUTES` | turns on the demo at `/demo`. A visitor gives a name, work email and company and gets an organization with a timer of this many minutes. The timer starts when the server is ready and ends by deleting it |
| `GUSTO_CLOUD_DEMO_EXTEND_MINUTES`, `GUSTO_CLOUD_DEMO_PATIENCE_MINUTES` | the demo's one extension (10), and how long a visitor waits to be ready (15) |
| `GUSTO_CLOUD_DEMO_MAX`, `GUSTO_CLOUD_DEMO_PER_DAY`, `GUSTO_CLOUD_DEMO_PER_PERSON`, `GUSTO_CLOUD_DEMO_PER_COMPANY` | the most demos at once (5), in a day (40), and that one address (2) and one company (6) may start in 24 hours |
| `GUSTO_CLOUD_BOOK_URL` | where a call is booked, linked from the end page |
| `GUSTO_CLOUD_DEMO_NOTIFY` | an address mailed when a visitor starts a demo: name, work email, company and time. Nothing is sent when unset |
| `GUSTO_CLOUD_LEAD_HOOK_URL`, `GUSTO_CLOUD_LEAD_HOOK_SECRET` | an https address that receives a lead as JSON when a demo starts and ends, and a secret sent in each request body. Both or neither. An unsent lead is kept under `leadhook/` and sent later. [deploy/google-sheet](../deploy/google-sheet/README.md) receives into a Google Sheet |
| `GUSTO_CLOUD_TURNSTILE_SITE`, `GUSTO_CLOUD_TURNSTILE_SECRET` | the Cloudflare Turnstile bot check on the demo form. The secret comes from the environment. Both or neither: one alone stops gusto cloud starting. A visitor who cannot be checked is turned away |
| `GUSTO_DATABASE_URL` for `gusto cloud` and `gusto operator` | where the accounts are kept: people, organizations, invitations, links, demos, leads, limits and each organization's runtime. Unset, it is `cloud.db` under `GUSTO_CLOUD_ROOT`. A `postgres://` address shares them between front doors, and its `search_path` names the schema, which is made when missing. The front door's address and password never go to an organization's server. [CLOUD-DATABASE.md](CLOUD-DATABASE.md) |
| `GUSTO_CLOUD_INDEX_DIR` | where gusto cloud has each organization's server keep its indexes when the organization's `GUSTO_DATABASE_URL` (in `GUSTO_CLOUD_ORG_ENV`) is not a Postgres address: `<dir>/<slug>/index.db`, on the local disk of the host the server runs on. `index` under `GUSTO_CLOUD_ROOT` when unset. Set it when the root is on a disk that hosts share |

The demo limits are starting values. The operator changes them on the operator page while gusto cloud runs. The operator page pauses demos. Run the tests with `go test ./... -race`. Gusto is Apache License 2.0 ([LICENSE](../LICENSE)). Third-party licenses are in [NOTICE](../NOTICE).
