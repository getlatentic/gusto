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
```

`inside` and `run` exit with the gate's answer: 0 when it passes.

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
  job.json          brief, repo, gate, harness, model, sensitivity, budget
  workspace/        the checkout the harness works in
  out/
    transcript.log  what the harness said
    harness.log     every line it printed, as it printed it
    stderr.log      what it complained about
    calls.jsonl     the model calls that reported tokens or cost
    commands.jsonl  written by an adapter that reports its tool calls
    diff.patch      what changed
    gate.log        the gate's whole output
    gates.json      the verdict
    manifest.json   a hash of each file above, the totals, and the signer
```

A token count or a cost is `null` when the harness does not report one. A zero
would read as free.

## Adapters

```bash
haas harnesses
```

| harness | program | needs |
| --- | --- | --- |
| claude | `claude` | the user's own account |
| codex | `codex` | the user's own account |
| opencode | `opencode` | nothing outside the building |

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

The three built-in adapters read their harness as plain text. A path is only
written here after a real run of that harness proved where the number sits.

## The container

```bash
docker build -t haas:dev .
HAAS_MODEL_URL=http://localhost:11434/v1 haas run /tmp/fix
```

`run` mounts the job directory, gives the container 1 CPU, 3 GiB and 512
processes, and destroys it afterwards. `HAAS_IMAGE` names a different image, and
`--build-arg HARNESS_PACKAGE` builds one for a different harness.

## Environment

| name | what it does |
| --- | --- |
| `HAAS_HARNESSES` | a directory of adapter files that override the built-in ones |
| `HAAS_MODEL_URL` | where a local model answers |
| `HAAS_IMAGE` | the runner image `haas run` starts |
| `HAAS_SIGNER` | who the manifest names |

## Tests

```bash
go test ./...
```

Ten tests. Six run a whole job with a fixture harness — a program that makes the
change, which is all our own definition of a harness requires — and cover the
cases the product rests on: a run whose manifest agrees with its own evidence, a
changed file reported, a file added after the run reported, a harness that only
claims to have finished failing the job, and a one-second budget stopping a
thirty-second harness.
