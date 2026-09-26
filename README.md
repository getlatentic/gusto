# haas

Harness as a service. A job is a directory, running it is a program, and the
result is files.

- **Any harness.** A harness is a program, and an adapter is a JSON file that
  says how to run it. Adding one is a file, not a release.
- **The gate decides.** A harness reporting success is a claim. The job passes
  when the repository's own command exits zero, and not before.
- **Evidence anyone can check.** Every run leaves a manifest of SHA-256 hashes.
  `haas verify` re-hashes the files and re-runs the gate, and needs nothing from
  the platform that produced them.
- **The runner goes in the box.** A harness with its own tool loop runs its
  edits and shell in its own process, so the container holds the runner.
- **One rule enforced today.** Restricted code only talks to a model in the same
  building. The runner refuses the job otherwise.

## Install

```bash
go build -o haas ./cmd/haas
```

## Run

A job needs a repository, a brief, and the command that says pass or fail. The
gate is everything after `--`, so quoting survives.

```bash
haas new /tmp/fix -repo /path/to/repo -brief "make the answer yes" -harness opencode -- go test ./...
haas inside /tmp/fix     # do the work here
haas run /tmp/fix        # do the work in a container
haas submit /tmp/fix     # send it to a server, which runs it
```

`haas serve` puts the pages in front of all of this: a form to describe a job, a
live view while it runs, its evidence and a bundle to take away. `haas submit`
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
and its manifest hash, at the commit the job ran against, pushes it to the job's
repository and opens a pull request naming the harness, the model, the gate and
the manifest's hash. It never merges. `-dry-run` prints the plan and does none
of it, and `-remote`, `-github owner/name` and `-base` override what it reads
off the job.

```
$ haas show /tmp/fix
brief    make the answer yes
harness  fixture
gate     sh -c test "$(cat answer.txt)" = yes
label    internal

gate     exit 0, passed true
harness  exit 0, stopped at the budget false
model    http://localhost:11434/v1
signer   tosin
files    7

$ haas verify /tmp/fix
7 files match, and the gate still exits 0
```

`verify` exits 1 and names the file when anything in `out/` has changed since
the run.

## A job directory

```
job/
  job.json          version, brief, repo, gate, harness, model, sensitivity, budget
  harnesses/        an adapter of your own, so the job runs anywhere
  workspace/        the checkout the harness works in
  out/
    transcript.log  what the harness said
    harness.log     every line it printed, as it printed it
    stderr.log      what it complained about
    calls.jsonl     the model calls that reported tokens or cost
    diff.patch      what changed
    gate.log        the gate's whole output
    gates.json      the verdict
    manifest.json   a hash of each file above, the totals, and the signer
```

A token count or a cost is `null` when the harness does not report one. A zero
would read as free.

`job.json` and `manifest.json` are published as versioned JSON Schemas in
[`schema/`](schema/). [docs/FORMATS.md](docs/FORMATS.md) says when the version
changes and what a reader does with one it does not know.

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
how to run one. A **configured harness** is a harness plus the skills and MCP
servers that shape it, and nothing here holds one yet. See
[docs/TERMS.md](docs/TERMS.md).


```bash
haas harnesses
```

| harness | program | needs | stream |
| --- | --- | --- | --- |
| claude | `claude` | the user's own account | measured |
| opencode | `opencode` | a model in the building | measured |
| codex | `codex` | the user's own account | plain text, not yet measured |

`needs` is one of `nothing`, `local`, `account` or `key`. `nothing` means exactly
that: a harness can be a program that makes the change, and then no model is
involved, no endpoint is checked, and a restricted job has nothing to refuse.

An adapter is a JSON file. A directory named by `HAAS_HARNESSES` overrides a
built-in adapter of the same name, so a customer corrects one or adds a harness
of their own without a new binary.

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

## The container

```bash
docker build -t haas:dev .
HAAS_MODEL_URL=http://localhost:11434/v1 haas run /tmp/fix
```

`run` mounts the job directory, gives the container 512 processes and whatever
`HAAS_JOB_CPUS` and `HAAS_JOB_MEMORY` allow, and destroys it afterwards. On a
machine that hosts anything else, set them: a limit larger than the machine is not
a limit. `HAAS_IMAGE` names a different image, and
`--build-arg HARNESS_PACKAGE` builds one for a different harness.

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

## Environment

| name | what it does |
| --- | --- |
| `HAAS_HARNESSES` | a directory of adapter files that override the built-in ones |
| `HAAS_MODEL_URL` | where a local model answers |
| `HAAS_IMAGE` | the runner image `haas run` starts |
| `HAAS_CONTAINER` | the container command, `docker` by default, `podman` for no daemon |
| `HAAS_JOB_MEMORY` | what one job may take, `3g` by default |
| `HAAS_JOB_CPUS` | how much CPU one job may take, `1` by default |
| `HAAS_SIGNER` | who the manifest names |
| `HAAS_GITHUB_TOKEN` | the token `haas ship` pushes to github.com and opens pull requests with; `HAAS_GITHUB_API` overrides the API address |
| `HAAS_SERVER` | where `haas submit` sends a job, with its scheme |
| `HAAS_ADDR` | where `haas serve` listens, `127.0.0.1:8040` by default |
| `HAAS_JOBS` | the directory `haas serve` keeps jobs in |
| `HAAS_PASSWORD` | the shared sign-in. `haas serve` refuses to start without it |
| `HAAS_JOB_CONCURRENCY` | how many jobs run at once |
| `HAAS_SANDBOX` | `here` runs a job in the server's own process instead of a container |

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
