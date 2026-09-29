# haas

Harness as a service. A job is a directory, running it is a program, and the
result is files.

- **Any harness.** A harness is a program, and an adapter is a JSON file that
  says how to run it. Adding one is a file, not a release.
- **The gate decides.** A harness reporting success is a claim. The job passes
  when the repository's own command exits zero, and not before.
- **Evidence anyone can check.** Every run leaves a manifest of SHA-256 hashes,
  signed with a key you hold when you give the runner one. `haas verify`
  re-hashes the files, names the signer from your keyring and re-runs the gate,
  and needs nothing from the platform that produced them.
- **The runner goes in the box.** A harness with its own tool loop runs its
  edits and shell in its own process, so the container holds the runner.
- **One rule enforced today.** Restricted code only talks to a model in the same
  building. The runner refuses the job otherwise.

What it is for, and what it is not yet, is in [docs/DIRECTION.md](docs/DIRECTION.md).

## Install

```bash
go build -o haas ./cmd/haas
```

To run the whole product, the pages included, on a machine or a cluster of your
own, [deploy/README.md](deploy/README.md) has a compose file and a Helm chart.

## Run

A job needs a repository, a brief, and the command that says pass or fail. The
gate is everything after `--`, so quoting survives.

```bash
haas new /tmp/fix -repo /path/to/repo -brief "make the answer yes" -harness opencode -- go test ./...
haas inside /tmp/fix     # do the work here
haas run /tmp/fix        # do the work in a container
haas submit /tmp/fix     # send it to a server, which runs it
```

`haas serve` puts the pages in front of all of this. A run happens on an
instance, a harness started from the catalog with a folder of its own: say
something to it in a thread, or give it a task with a gate, the team its spend
counts against, a cap and how sensitive the code is. A one-off against a
repository is an instance started with that repository. `/jobs` is the record of
every run, whatever started it: its verdict, its cost, the instance it ran on, a
live view while it runs, and its evidence and a bundle to take away. `haas submit`
posts a job directory to that server over `POST /api/jobs`, carrying an adapter
the job holds so a server that has never heard of your harness can still run it.

Every request `haas serve` answers leaves one JSON line on stderr, where journald
keeps it: method, path, status, bytes, duration, the client's address, and
whether the caller came with a `cookie`, a `bearer` password, or `none`. It names
the kind of credential and never its value, and it leaves out the query string
and `/healthz`.

`inside` and `run` exit with the gate's answer: 0 when it passes.

`haas ship /tmp/fix` hands a passed job back as a pull request. It re-hashes the
evidence first and refuses, with exit 1, a job whose gate failed or whose files
changed since the run. It puts `out/diff.patch` on a branch named for the job
and its manifest hash, at the commit the job ran against. It commits `job.json`
and `out/`, byte for byte, on top of that commit under
`refs/haas/evidence/<manifest sha256>`, a ref that is not a branch, so the
evidence never enters the change under review. It pushes both to the job's
repository and opens a pull request naming the harness, the model and the
gate, linking the manifest and the transcript, and printing the one command
that checks them. It never merges. `-dry-run` prints the plan and does none of
it, and `-remote`, `-github owner/name` and `-base` override what it reads off
the job.

A reviewer runs the command the pull request prints, which has this form:

```bash
haas check <repository> <manifest sha256>
```

`check` fetches the evidence ref with the reviewer's own git credentials, holds
`manifest.json` to the hash the pull request names, re-hashes every file,
rebuilds the workspace from `diff.patch` and checks it is the pull request's
commit, then runs the gate. Like `verify`, it exits 1 and names what does not
agree, and it runs nothing when the files do not match. `-into <dir>` keeps the
job directory it unpacked.

```
$ haas show /tmp/fix
brief    make the answer yes
harness  fixture
gate     sh -c test "$(cat answer.txt)" = yes
label    internal

gate     exit 0, passed true
harness  exit 0, stopped at the budget false
model    http://localhost:11434/v1
signer   acme-2026
files    7

$ haas verify /tmp/fix
signed by acme-2026 (key sha256:7da853821a143b6aeab0bbdff028a1ea)
7 files match, and the gate still exits 0
```

`verify` exits 1 and names the file when anything in `out/` has changed since
the run.

### Signing the record

Hashes show a file changed; they do not stop whoever stores the record from
rewriting all of it, hashes included. A signature does, as long as the key is
somewhere they are not.

```bash
haas keygen acme-2026 -dir ~/.haas     # acme-2026.key (0600) and acme-2026.pub
export HAAS_SIGNING_KEY=~/.haas/acme-2026.key   # on the machine that runs jobs
export HAAS_KEYRING=~/keyring                   # on the machine that verifies
cp ~/.haas/acme-2026.pub ~/keyring/
```

To replace a key, back one up, or keep it in AWS KMS, see
[docs/KEYS.md](docs/KEYS.md). `haas keys` lists which key signed each record and
which records no key in the keyring can vouch for.

The runner signs the exact bytes of `out/manifest.json` with Ed25519 and writes
the signature beside it, as `out/manifest.sig`. `haas inside` signs in its own
process. `haas run` keeps the key on the host and signs after the container is
gone, so the key never goes in with the agent. With no key the run still works,
and the record is unsigned.

`verify` names the signer from the keyring: the `.pub` file whose key made the
signature, never the manifest's `signer` field, which is the runner's own word.

`show`, the job page `serve` draws, `ship`'s pull request body and `export`'s
`audit.json` name the signer the same way, and where the keyring cannot vouch for
a name they say so in the same words rather than repeat the manifest's claim:

| the record | `show`, the job page, the pull request and `audit.json` say |
| --- | --- |
| signed by a key in the keyring | `<name>` |
| signed, with no `HAAS_KEYRING` | `signed, not checked: no keyring` |
| signed by a key not in the keyring | `signed, not checked: key <id> is not in the keyring` |
| signed, then changed | `signed, and the signature does not hold` |
| unsigned | `unsigned` |

| the record | `verify` says | exit |
| --- | --- | --- |
| signed by a key in the keyring | `signed by <name> (key <id>)` | the gate's |
| signed by a key not in the keyring | `out/manifest.sig: it was made by key <id>, which is not in the keyring` | 1 |
| signed, then changed | `manifest.sig does not match manifest.json` | 1 |
| unsigned | `unsigned: no key stands behind this record` | the gate's, or 1 with `-signed` |

An unsigned record passes unless `-signed` is given, because every record made
before signing is unsigned and still has to verify. To rotate, make a new key,
add its `.pub` to every keyring and keep the old `.pub` there: each signature
names its key by id, so what the old key signed still verifies after its private
half is destroyed.

## Replay

`haas replay` runs finished jobs again, unchanged, and compares the new verdict
with the old one. It is how a harness upgrade that makes things worse is caught
before a team notices.

```
$ haas replay /jobs -here
job        harness  before      after       harness exit  change  replay
/jobs/one  yes      passed (0)  passed (0)  0 -> 0        same    /jobs/one-replay-1
/jobs/two  yes      failed (1)  failed (1)  0 -> 0        same    /jobs/two-replay-1

2 replayed: 0 worse, 0 better, 2 same, 0 could not run
```

It takes job directories, or a directory of jobs, and makes a fresh directory
beside each, `<job>-replay-<n>`, or under `-into`. The new job is the old
`job.json` and adapter with one change: `ref` becomes the commit the old run
started from, so a repository that moved on since does not change the question.
It runs in a container like `haas run`, or in this process with `-here`, and
leaves evidence of its own. The old directory is only read, and a job whose
evidence no longer checks is not replayed, because its verdict is not the one to
compare against.

The commit is read off the old workspace, and not from its `HEAD`: a harness
that commits moves `HEAD` onto a commit only its workspace has. The clone's
`refs/remotes/origin/*` and `refs/tags/*` keep what the repository held when the
job cloned, so the job's `ref` is resolved against those, and a `ref` that is
already a commit id is its own answer.

A replay says so in `out/replay.json`: the original's directory, the hash of its
manifest, the commit and the reference it was read from, and how many commits the
original harness made on top. It is written before the run, so the replay's
manifest hashes it. The harness can read which run it repeats and never that
run's verdict.

The exit code is 1 when any verdict got worse, 2 when a job could not be
replayed, and 0 otherwise.

## Compare

`haas compare` runs one job on two or more harnesses, and keeps the runs as one
comparison: a directory holding a job directory a harness and a
`comparison.json` naming them.

```
$ haas compare /tmp/fix yes maybe -here
comparison  /tmp/fix-compare-1
commit      38b04cbd95cc3b008ce9f5cdee6d5357e0460f8d
held        brief, budget, gate, ref, repo, sensitivity, version

harness  verdict     harness exit  at budget  tokens  cost  evidence
yes      passed (0)  0             false      -       -     checks
maybe    failed (1)  0             false      -       -     checks
```

The runs differ only by harness. The commit is pinned once, the way `replay`
pins one, before either run clones, and the brief, gate, budget and model are
copied. Each run has its own workspace and evidence, and in a container neither
can see the other. `haas compare /tmp/fix-compare-1` prints the comparison again
from the evidence. What is held constant and why is in
[docs/COMPARE.md](docs/COMPARE.md).

## A job directory

```
job/
  job.json          version, brief, repo, gate, harness, model, sensitivity, budget, team
  harnesses/        a configured harness of your own, so the job runs anywhere
  workspace/        the checkout the harness works in
  out/
    transcript.log  what the harness said
    harness.log     every line it printed, as it printed it
    stderr.log      what it complained about
    calls.jsonl     the model calls that reported tokens or cost
    proxy.jsonl     every call that left through the proxy, and what it cost
    commands.jsonl  what the harness ran, apart from what it said
    diff.patch      what changed
    gate.log        the gate's whole output
    gates.json      the verdict
    manifest.json   a hash of each file above, the totals, and the signer
    manifest.sig    the signature over manifest.json, when the runner has a key
```

A token count or a cost is `null` when the harness does not report one. A zero
would read as free.

`job.json`, `manifest.json` and `manifest.sig` are published as versioned JSON Schemas in
[`schema/`](schema/). [docs/FORMATS.md](docs/FORMATS.md) says when the version
changes and what a reader does with one it does not know.

## An audit export

`haas export <dir>` writes one job's audit record into a new directory,
`<dir>.audit` unless `-to` names another. It answers who asked, what ran, what
changed, which model, how much it spent and who approved it, in `audit.json`, and
it carries the files each answer comes from.

```
job.audit/
  audit.json        the summary, derived only from the files below
  job.json          as the run left it, unless the policy redacted it
  state.json        the server's state for the job, when it has one
  requests.jsonl    the request log's lines that name the job, with -requests
  out/              the evidence and its manifest
```

The export is a job directory without a workspace, so the manifest check that
verifies a job verifies it too. `haas export -verify job.audit` runs that check,
then checks every file against the hash the export recorded, then derives the
summary again and compares it with `audit.json`. It exits 1 and names the file
when anything disagrees. `haas verify` also re-runs the gate, which needs the
workspace an export leaves behind.

Where the platform keeps nothing, the summary says `not recorded` rather than
leaving the field out. Two answers are always that today: who asked, because
everybody signs in with one shared password, and who approved, because an agent
opens a pull request and a person merges it on the forge.

`-policy <file>` says what an export retains and what it hides. It is JSON:

```json
{
  "drop": ["out/transcript.log", "requests.jsonl"],
  "redact": {
    "fields": ["brief", "remote"],
    "patterns": [{"name": "api-key", "match": "sk-[A-Za-z0-9-]{16,}"}]
  }
}
```

A dropped file is not carried. A field's value becomes `"[redacted]"` in every
JSON and JSON-lines file, and a pattern's match becomes `[redacted: api-key]`
anywhere in text. `audit.json` lists each file the policy changed, the rules that
changed it, and the hash the manifest holds for the original, and it records the
policy whole. Verification expects exactly those files to differ, so what the
policy left alone still verifies against the run. [docs/FORMATS.md](docs/FORMATS.md#the-audit-export)
has the format.

## What a job costs, measured

One task, run three ways: a one-line fix to a shell function, with a shell script
as the gate.

| harness and model | finished | wall clock | billed tokens | cost |
| --- | --- | --- | --- | --- |
| `claude`, Opus 5 | yes | 2m55s | 1,134,184 | $1.3161 |
| `opencode`, `ollama/gpt-oss:20b` | no | 2m01s | 16,669 | $0 |
| `opencode`, `ollama/llama3.2:3b` | no | 1m14s | 30,138 | $0 |

Claude's total is 1,065,920 cache reads across 41 turns, and 61,508 cache writes.
The task is not what costs money; the turns are. Neither local model finished:
`gpt-oss:20b` searched for the file with an empty path argument, found nothing and
stopped, and `llama3.2:3b` invented a command and then emitted a tool call as
plain text. The gate refused both, which is what it is for.

One trivial task is not a benchmark. It is the number this repository can prove,
and every figure above comes from a manifest in a job directory.

### Measuring it yourself

`haas matrix` runs a fixed set of jobs on every harness and model pair a plan
names, keeps each run as an ordinary job directory, and writes the table in the
shape above, with the place it ran as a column. A cell that finished is never
run again unless asked, because each one costs money.

```bash
haas new one-line-fix -repo /path/to/repo -brief "fix the operator" -- sh test.sh
cp docs/matrix/issue-8.json plan.json          # opencode on two local models
haas matrix plan.json -here                    # plan-matrix/table.md and table.json
HAAS_MATRIX=plan-matrix/table.json haas serve  # mark a pair that cannot finish it
```

[docs/MATRIX.md](docs/MATRIX.md) has the plan, the layout and the columns.

### What a job needs from the machine

An `opencode` job peaked at **662.7 MiB** in its container, sampled with
`docker stats` while it ran, with the model outside the container. That is the
harness's own runtime. Size a runner by the jobs it should hold at once, not by
the image: roughly 0.7 GiB a concurrent job, plus the container runtime, plus a
Go binary in the tens of megabytes.

Docker Desktop on a Mac costs about 2 GiB because it runs a Linux virtual
machine. On Linux there is no virtual machine, and `podman` needs no daemon at
all — set `HAAS_CONTAINER=podman`.

## Adapters

A **harness** is the program — `claude`, `codex`, `opencode`. An **adapter** is
how to run one. A **configured harness** is the adapter's directory, with the
prompt and model policy that shape the harness, and the setup of skills and MCP
servers a job gives it. Every run names the one it ran by digest. See [docs/TERMS.md](docs/TERMS.md).


```bash
haas harnesses
```

| harness | program | needs | stream |
| --- | --- | --- | --- |
| claude | `claude` | the user's own account | measured |
| opencode | `opencode` | a model in the building | measured |
| codex | `codex` | the user's own account | measured |

`needs` is one of `nothing`, `local`, `account` or `key`. `nothing` means exactly
that: a harness can be a program that makes the change, and then no model is
involved, no endpoint is checked, and a restricted job has nothing to refuse.

An adapter is a JSON file. A directory named by `HAAS_HARNESSES` overrides a
built-in adapter of the same name, so a customer corrects one or adds a harness
of their own without a new binary. It holds bare adapters, `name.json`, or
configured harnesses, `name/adapter.json` with the files that shape it beside it,
which the adapter reaches through `{harnessDir}`.

```json
{
  "name": "opencode",
  "program": "opencode",
  "argv": ["run", "{brief}"],
  "needs": "nothing",
  "stream": "plain"
}
```

`{brief}`, `{model}` and `{modelUrl}` are filled from the job. A `jsonl` stream
adds dotted paths — `text`, `inputTokens`, `outputTokens`, `costUsd` — and
`totals` says whether the harness reports per call or as a running total.
Summing a running total counts every call more than once.

A path is only written here after a real run of that harness proved where the
number sits. A harness whose stream has not been measured is read as plain text,
which keeps the transcript and the patch and leaves the numbers out rather than
inventing them.

Cache tokens are counted, because they are billed. Leaving them out understated
one real run by 168 times.

## The way out of a sandbox

A harness needs a model and a model needs a credential. Putting that credential
in the container is the one thing this refuses to do, so the container is given
an address instead:

```
container   HAAS_MODEL_URL=http://127.0.0.1:8041/model/v1, the job's token as its key
runner      relays 127.0.0.1:8041 in the box to the proxy's socket, mounted in
proxy       validates the token, adds the real credential, writes down what went
```

The token is minted per job and dropped when the job ends, so it is worth nothing
to anybody who takes it afterwards, and what a job spends is attributed to the
job rather than to an account. Whatever authorization a container sends is
discarded rather than forwarded.

The proxy writes `out/proxy.jsonl`: what actually left the box, independent of
what the harness says it spent. A harness reporting its own usage is a claim;
this is the count. Comparing the two is how a harness that under-reports is
caught.

## Nothing else leaves

A container with a proxy runs with `--network=none`. It has no route anywhere,
so the only way out is the proxy's unix socket, mounted in at
`/run/haas/proxy.sock`, which the runner inside relays to from loopback. The
repository is cloned before the container starts, so the box needs no route to
it either.

Every host but the model is refused unless `HAAS_ALLOWED_HOSTS` names it. The
container is given `HTTPS_PROXY` and `HTTP_PROXY` pointing at the relay, with the
job's token as the password, so a package manager or a fetch goes through the
proxy the ordinary way:

```bash
HAAS_ALLOWED_HOSTS=registry.npmjs.org,proxy.golang.org,*.githubusercontent.com
```

An entry is a host, which allows its ports 80 and 443, or `host:port` for one
port. `*.example.com` allows every name under the domain and not the domain
itself. Nothing set is nothing allowed.

Every request, sent or refused, is a line in `out/proxy.jsonl` with its `host`,
and a refused one says `denied`, so the manifest's hash covers the attempt. The
proxy answers a refused host with 403 and sends nothing on. An HTTPS request is
carried as a tunnel, so the proxy sees the host and not what was said.

Two things are **not yet measured**. Whether `--network=none` with a mounted
socket holds on the box's podman, rootless or not, and on Docker, has to be
checked there; the tests hold the command line and not the runtime. And Docker
Desktop on a Mac may not hand a container a host socket at all. Set
`HAAS_JOB_NETWORK=open` for the old route, where the container keeps the
runtime's network and reaches the proxy on `HAAS_PROXY_ADDR`. That route is not
sealed: the allowlist governs what goes through the proxy, and nothing stops a
container that ignores it.

## What a job may spend

The proxy counts what each call cost from the provider's own answer, never from
what the harness says. It reads the OpenAI-compatible `usage` object,
`prompt_tokens` and `completion_tokens`, from a plain answer and from the last
event of a streamed one. A streamed request is sent on with
`stream_options.include_usage` set, because a stream carries no usage otherwise.
That path is written against the documented shape and has **not yet been
measured against a real upstream**; the tests hold fixtures, labelled as such.

Money needs prices, and prices are the operator's. `HAAS_SPEND_POLICY` names a
file of them, with the monthly cap on each team:

```json
{
  "prices": { "some-model": { "inputPerMillion": 2.5, "outputPerMillion": 10 } },
  "teams": { "platform": { "usdPerMonth": 200 } }
}
```

The numbers above show the format and are nobody's price. A field the file
names that this build does not know stops the server, because a misspelt cap
would otherwise be no cap.

Each line of `out/proxy.jsonl` carries `inputTokens`, `outputTokens` and `usd`.
An answer with no usage, or a model with no price, leaves them `null`: unmeasured,
not free.

There are two caps:

- **A job's own**, `budget.usd` in `job.json`. Left out is no cap; zero allows
  nothing.
- **A team's**, per calendar month in UTC, from the policy. A job counts against
  the team its `team` field names. That is a label the person asking chooses:
  nothing proves they belong to the team yet, so a team cap holds against
  mistakes and not against somebody determined to spend.

The proxy checks both before each call leaves. Once one is spent it answers
`402` with the reason, in the error shape an OpenAI-compatible client prints, and
writes the refusal to `out/proxy.jsonl`. A call already in flight when the cap is
reached finishes, so a job can end above its cap by the calls it had in flight,
which is one for a harness that asks one at a time. A cap it cannot count fails
closed: under a cap, a model with no price is refused before anything is sent,
and a successful answer that carried no usage stops the job's next call. An error
answer with no usage is recorded as unmeasured and does not stop it.

The refusal does not kill the harness. It takes away the model, and the time
budget ends a harness that does not stop by itself. The manifest then records
`stoppedByCap`, naming the cap, the team, the limit and what was spent, and the
job page and `haas show` say which cap stopped it:

```
stopped  by the team cap: team platform has spent $200.0312 of its $200.0000 cap for 2026-09
```

That line shows the format; it is not from a run.

A server that restarts rebuilds every team's month from the jobs' own
`out/proxy.jsonl`, so the lines in the evidence are the ledger. A job run again in
the same directory keeps what it spent before.

## What the agents cost

`haas spend` says what a month cost, by team, by project and by job, and the
`spend` page in `haas serve` says the same. `GET /api/spend?month=2026-09` hands a
script the same report as JSON, behind the password `POST /api/jobs` takes.

```bash
haas spend                   # this month, from HAAS_JOBS
haas spend -month 2026-08    # another month
haas spend -json             # for a script
```

The report reads the ledger the caps are checked against, so a report and a cap
cannot disagree: the server's page reads the ledger the proxy counts in, and the
command rebuilds it from the same `out/proxy.jsonl` lines through the same
counting. A project is the repository a job worked in. A job that named no team
is reported as having none.

Unmeasured spend is never shown as free. A call whose answer carried no usage, or
whose model has no price, is counted as unmeasured, and a sum that includes one
reads `at least $…`, or `unmeasured` when nothing in it was measured.

The report has **not been reconciled with a provider's invoice**. That needs a
month of real calls on a real account, which nobody has run yet, so the claim
that the totals match what the provider bills is still open.

Without `HAAS_MODEL_UPSTREAM` there is no proxy: the container is given the
model's address directly, which is the local case where there is no credential
to keep out of anywhere.

## The container

```bash
docker build -t haas:dev .
HAAS_MODEL_URL=http://localhost:11434/v1 haas run /tmp/fix
```

`run` mounts the job directory, gives the container 512 processes and whatever
`HAAS_JOB_CPUS` and `HAAS_JOB_MEMORY` allow, and destroys it afterwards. On a
machine that hosts anything else, set them: a limit larger than the machine is not
a limit. `HAAS_IMAGE` names a different image.

One image carries several harnesses. `HARNESSES` lists them as `name=package`,
the adapter's name beside the npm package that provides its program, and the
default is `opencode=opencode-ai`:

```bash
docker build -t haas:dev \
  --build-arg HARNESSES="opencode=opencode-ai claude=@anthropic-ai/claude-code codex=@openai/codex" .
docker image inspect --format '{{ index .Config.Labels "com.getlatentic.haas.harnesses" }}' haas:dev
```

The image says what it carries in that label, and `haas serve` reads it when it
starts. It still asks the image's PATH for each program, because the label is a
claim and the PATH is what a job meets, and it offers only the harnesses it
finds. A job that names one the image lacks anyway, from the command line or
from a server whose image changed under it, is refused by the runner inside the
box before the harness starts. The job ends `failed`, and its transcript says
which program is missing and how to carry it.

Each harness's own configuration is a directory under `image/harness/`, named
after it and copied to `/root/.config/<name>/`. opencode reads that directory
already, and the image points claude and codex at theirs with
`CLAUDE_CONFIG_DIR` and `CODEX_HOME`.

The runner clones the repository before the container starts, so a repository
credential stays outside the box with the agent. An adapter of your own is copied
into the job by `haas new`, so a container that has never heard of your harness
still runs it.

Two things were measured rather than assumed, and both fail silently otherwise:

- A bind mount of a path the container runtime does not share hands the container
  an **empty directory**. `run` refuses by name instead. On this machine Docker
  shares `/Users` and not `/private/tmp`.
- Docker Desktop already resolves `host.docker.internal`, and to the right
  address. Adding `--add-host=host.docker.internal:host-gateway` there replaces
  192.168.5.2, which is the host, with 172.17.0.1, which is the virtual machine.
  `run` adds the alias only where the name does not already resolve, which is
  plain Linux Docker.

A model on the host has to listen on more than loopback for a container to reach
it. With Ollama that is `OLLAMA_HOST=0.0.0.0` and a restart; until then `haas`
stops before starting the harness and says no model answers at that address.

## Before a job

```bash
haas doctor
```

`doctor` checks what a job needs from this machine, one named line a check, and
exits 1 when any fails, so a deploy stops on it rather than on its first job. A
failure says what to do on the line under it; a check that does not apply says
`skip` and why.

It asks the container command whether it answers, looks for the image locally,
starts a container under the job's own `--memory` and `--cpus`, and mounts the
jobs directory to see a file it left there. Each of those fails a job with exit
125 or an empty directory otherwise, and a cap the runtime refuses is only
refused at a start. It checks rootless podman reaches its runtime directory,
which a systemd `ProtectHome` hides, and that `git` is on PATH, `HAAS_JOBS` is
writable, `HAAS_MODEL_URL` answers and `HAAS_HARNESSES` parses when they are
set. It says whether `HAAS_PASSWORD` is set, never what it is.

```
$ haas doctor
skip  podman runtime directory  the container command is docker, not podman
fail  container command         docker does not answer: failed to connect to the docker API at unix:///var/run/docker.sock; ...
                                fix: start the docker daemon, or for rootless podman read the runtime directory line above
skip  image                     the container command check did not pass
skip  job limits                the image check did not pass
pass  git                       git is at /usr/bin/git
```

## Environment

| name | what it does |
| --- | --- |
| `HAAS_HARNESSES` | a directory of adapter files that override the built-in ones |
| `HAAS_MODEL_URL` | where a local model answers |
| `HAAS_IMAGE` | the runner image `haas run` starts |
| `HAAS_CONTAINER` | the container command, `docker` by default, `podman` for no daemon |
| `HAAS_JOB_MEMORY` | what one job may take, `3g` by default |
| `HAAS_JOB_CPUS` | how much CPU one job may take, `1` by default |
| `HAAS_SIGNER` | who the manifest names. The runner's own word; every surface names the signer from the keyring |
| `HAAS_SIGNING_KEY` | the private key the runner signs manifests with, from `haas keygen`, or `awskms://<key>` for a key AWS KMS holds. Kept on the host, never in a job directory or a container. [docs/KEYS.md](docs/KEYS.md) covers rotation, backup and KMS |
| `HAAS_CHANGES` | the file the record of what changes on the platform is appended to: sign-ins, refusals, role, setup and settings changes. Hash-chained, and signed when a key is set. `haas changes verify` checks it. [docs/CHANGES.md](docs/CHANGES.md) |
| `HAAS_KEYRING` | a directory of `<name>.pub` public keys `verify`, `show`, `serve`, `ship` and `export` trust, retired ones included |
| `HAAS_GITHUB_TOKEN` | the token `haas ship` pushes to github.com and opens pull requests with; `HAAS_GITHUB_API` overrides the API address |
| `HAAS_GITHUB_APP_ID`, `HAAS_GITHUB_APP_KEY` | a GitHub App that reaches the repositories jobs name. Each job gets a token for one repository, revoked when its work is done, and the box never holds it. Set both or neither. [docs/REPOS.md](docs/REPOS.md) |
| `HAAS_GITHUB_URL` | a GitHub Enterprise Server's address, with `HAAS_GITHUB_API`; github.com when unset |
| `HAAS_SERVER` | where `haas submit` sends a job, with its scheme |
| `HAAS_ADDR` | where `haas serve` listens, `127.0.0.1:8040` by default |
| `HAAS_JOBS` | the directory `haas serve` keeps jobs in |
| `HAAS_PASSWORD` | the shared sign-in. `haas serve` refuses to start without it |
| `HAAS_JOB_CONCURRENCY` | how many jobs run at once |
| `HAAS_MODELS` | the models the pages offer, comma separated. Only a deployment knows which its credential reaches |
| `HAAS_SANDBOX` | `here` runs a job in the server's own process instead of a container |
| `HAAS_MODEL_UPSTREAM` | the model's root address, with no version on it. Setting it turns the proxy on |
| `HAAS_MODEL_KEY` | the credential the proxy adds. It never reaches a container |
| `HAAS_MODEL_NAME` | what the evidence calls that credential |
| `HAAS_PROXY_SOCKET` | the socket a sealed container reaches the proxy through, `.proxy.sock` in `HAAS_JOBS` by default |
| `HAAS_ALLOWED_HOSTS` | the hosts besides the model a job may reach, comma-separated. Nothing set is nothing allowed |
| `HAAS_JOB_NETWORK` | `open` gives containers the runtime's network and the proxy on `HAAS_PROXY_ADDR` instead of sealing them |
| `HAAS_PROXY_ADDR` | where the proxy listens when `HAAS_JOB_NETWORK=open`, `0.0.0.0:8041` by default |
| `HAAS_SPEND_POLICY` | a file of model prices and team caps the proxy counts with. Without it every model is unpriced and no team is capped |

## Tests

```bash
go test ./... -race
```

Six of them run a whole job with a fixture harness — a program that makes the
change, which is all our own definition of a harness requires — and cover the
cases the product rests on: a run whose manifest agrees with its own evidence, a
changed file reported, a file added after the run reported, a harness that only
claims to have finished failing the job, and a one-second budget stopping a
thirty-second harness.
