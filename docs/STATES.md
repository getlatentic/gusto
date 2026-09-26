# A job's states

A job's state is a file. `state.json` sits in the job directory beside the
evidence, so the directory is the whole story of the job and nothing is left
behind in a database. The index in memory is a cache of that file, rebuilt by
reading the directory (`internal/jobs/state.go:1-8`).

There are five states and four writers. The writers are `Open`, `Add`, `Claim`
and `Finish`, all in `internal/jobs/index.go`. Nothing else writes the file.

## The states

| state | what it says | written by |
| --- | --- | --- |
| `queued` | the job is on disk and nobody has taken it | `Add`, `index.go:55-66` |
| `running` | a worker holds it | `Claim`, `index.go:89-113` |
| `passed` | the gate exited 0 | `Finish`, `index.go:116-131` |
| `failed` | the gate exited non-zero, or the job could not run | `Finish`, `index.go:116-131` |
| `interrupted` | the file said `running` when a server started | `Open`, `index.go:40-46` |

`passed`, `failed` and `interrupted` are final. `Done` reports all three, and
nothing reads a final state and queues the job again
(`internal/jobs/state.go:29-31`).

A directory with no `state.json` is not a job the index knows, and `Open` skips
it (`index.go:36-39`). `haas new` writes `job.json` and the adapter and no
`state.json`, so a job made on the command line inside the server's jobs
directory is invisible to the server.

## The transitions

| from | to | what causes it | what the file gains |
| --- | --- | --- | --- |
| — | `queued` | the form is accepted (`internal/web/pages.go:95-103`) | every field, `queuedAt` |
| `queued` | `running` | a worker takes the oldest queued job (`index.go:89-113`) | `startedAt` |
| `running` | `passed` | the work returns exit 0 (`internal/jobs/queue.go:88`) | `exit` 0, `endedAt` |
| `running` | `failed` | the work returns a non-zero exit (`queue.go:93-95`) | that `exit`, `endedAt` |
| `running` | `failed` | the work returns an error (`queue.go:90-92`) | `exit` -1, `endedAt` |
| `running` | `interrupted` | a server starts and reads `running` (`index.go:40-46`) | `endedAt` |

`Claim` picks the queued job with the lowest `queuedAt` under the index mutex,
so two workers in one process cannot take the same job (`index.go:89-113`).

## What reaches the disk while a job runs

`Claim` writes `state.json` before the work starts. Everything after that is
written by the runner inside the box, in this order
(`internal/runner/inside.go:26-97`):

| written | when | by |
| --- | --- | --- |
| `out/harness.log`, `out/transcript.log`, `out/calls.jsonl`, `out/stderr.log` | before the harness starts, truncated | `record.go:30-48` |
| a line in each | as the harness prints it | `inside.go:128-138` |
| `out/diff.patch` | after the harness stops | `inside.go:67` |
| `out/gate.log`, `out/gates.json` | after the gate answers | `runner/gate.go:31-44` |
| `out/manifest.json` | last, hashing everything above | `inside.go:78-92` |

Each line is on the bind-mounted directory before the next one is read, because
a container's own disk does not survive the container (`record.go:12-14`).

The runner refuses the job before it writes anything when a restricted job names
a model that is not local (`inside.go:36`), or when a harness needing a local
model has none answering (`inside.go:39-43`). Neither leaves a gate verdict. In
the box the refusal reaches the queue as exit 2; in the server's own process it
reaches the queue as an error.

The budget that ends a harness is the time budget. `Inside` builds a context
from `budget.seconds`, which defaults to 900 (`inside.go:55-56`,
`internal/job/job.go:85`). `budget.turns` defaults to 40 and nothing reads it.

## A server restart

| how the server stops | what happens to the job in flight |
| --- | --- |
| SIGINT or SIGTERM | the job runs on, and `Finish` records it if it ends within 60 seconds |
| SIGKILL, a crash, the machine | the container runs to its end and nothing records it |

On a signal the handler shuts the HTTP server down with a 30-second grace
(`cmd/haas/serve.go:53-60`). `queue.Stop()` then closes the stop channel and
waits for every worker (`queue.go:54-57`). A worker inside `q.work` is not
interrupted, so the stop takes as long as the job does. The service unit gives
the whole thing 60 seconds and then kills everything in the cgroup
(`deploy/ansible/roles/serve/templates/haas.service.j2:23-24`). A job with the
default budget may take 900 seconds, so in practice most jobs meet the hard kill
rather than the clean stop. The comment above those two lines says the service
gets time to end its jobs and write their evidence before anything is killed
(`haas.service.j2:21-22`). That holds for a job of under a minute and for no
other.

After a hard kill the file still says `running`. The container is a child of the
container daemon, not of `haas`, so it keeps going. Measured: a server killed
with SIGKILL 12 seconds into a harness that prints for 15 left the container
running, and the container finished the harness, ran the gate, and wrote all
seven evidence files plus `out/manifest.json` saying `"passed": true`.
`state.json` still said `running` with no `endedAt`.

A job running in the server's own process dies with the server instead, because
`HAAS_SANDBOX=here` makes the work a function call rather than a container
(`cmd/haas/serve.go:70-86`).

The next `Open` then writes `interrupted` over that job (`index.go:40-46`). On
the same job directory afterwards:

```
$ haas verify <the job>
7 files match, and the gate still exits 0
```

and the job page shows `State: interrupted` beside `Gate: exit 0, passed true`.
`Open` reads `state.json` and never `out/manifest.json`, so a job that finished
and a job that died are the same to it.

A job left `queued` by a stopped server stays `queued`, and the next server takes
it. Measured: a job directory with a `queued` `state.json`, present before the
server starts, ends `passed`.

## A runner that dies mid-job

| what dies | the harness line in the manifest | the job's state |
| --- | --- | --- |
| the harness, on its own non-zero exit | `harnessExit` is that code | the gate decides |
| the harness, by a signal | `harnessExit` -1 | the gate decides |
| the harness, at the time budget | `harnessExit` -1, `stoppedAtBudget` true | the gate decides |
| the runner, PID 1 in the box | no manifest | `failed`, with the container's exit, 137 for a kill |
| the runner, on an error before the gate | no manifest | `failed`, `exit` 2 in the box, -1 on the host |
| `haas serve` | written by the container that outlives it | `interrupted` |

Measured, with a harness that kills itself and a gate that passes: the manifest
says `harnessExit: -1`, `stoppedAtBudget: false`, `gate: {code: 0, passed:
true}`, and `haas inside` exits 0. A killed harness does not fail a job. The
gate decides, on whatever the harness left in the workspace.

`stoppedAtBudget` separates the budget from every other signal
(`inside.go:84`). Nothing else about the signal is kept: `ExitCode()` is -1 for
any of them.

The fifth row has two answers because the boundary is in the middle of it. Inside
the box, `haas inside` turns an error into exit 2 (`cmd/haas/main.go:122-132`,
`main.go:228-231`), the container carries that out, and the queue reads it as a
non-zero exit (`queue.go:93-95`). On the host — reading `job.json`, cloning the
repository, starting the container — the error reaches the queue as an error and
becomes `exit` -1 (`queue.go:90-92`). Measured on the container path: a job
naming a harness the image does not carry ends `failed` with `exit` 2, and a job
whose repository does not exist ends `failed` with `exit` -1.

## What a watcher sees

`/watch/{id}` tails `out/transcript.log` every 300 milliseconds and asks the
index whether the job has ended (`internal/web/stream.go:19-58`). The file is the
stream, so a watcher who arrives late is caught up from the beginning.

| the job is | the watcher gets |
| --- | --- |
| `queued` | an open stream and no data, so the page reads `waiting…` |
| `running` | each line, in the order the runner wrote it |
| `passed`, `failed` | one last read of the file, then `event: ended` with the state |
| `interrupted` | the transcript up to the kill, then `event: ended` with `interrupted` |

Measured on an interrupted job: the stream replays the transcript and then sends
`event: ended` / `data: interrupted`.

A watcher across a restart is not told. The cookie key is made at boot
(`internal/web/auth.go:24-32`), so every cookie from the old process is invalid
and the reconnect is answered with 303 to `/login` — measured. The page opens a
stream only when the state it was rendered with is `queued` or `running`, and it
listens for `ended` and for messages and not for errors
(`internal/web/templates/job.html:38-56`). The person reloads, signs in, and the
page then says `interrupted`.

## What is not decided yet

### An interrupted job is never retried

Nothing retries it, and `interrupted` is final (`state.go:29-31`). That is the
right default while the state means what it means here, but it is not a decision
anybody made about retrying: it follows from `Open` being the only writer of the
state and the queue only ever claiming `queued`.

Three things are true of an interrupted job, and a fourth is a bug standing in
front of the question.

**The container is gone.** `sandbox.Run` passes `--rm`, so the box a job ran in
does not survive it (`internal/sandbox/sandbox.go:45`). A retry is a new
container. Nothing is carried over, which is the easy part.

**The workspace may be half-edited.** `Fill` leaves a directory that already
holds `.git` exactly as it is (`internal/runner/workspace.go:14-17`). A retry
therefore runs the harness over the previous attempt's edits, against a brief
written for the original tree. A safe retry resets the workspace to a named
commit first, or clones again — and the job has to say which, because "keep what
the agent did and carry on" and "start over" are different products.

**The evidence may be partial.** `Open` creates the four log files with
`os.Create`, which truncates (`internal/runner/record.go:41`). A retry overwrites
the interrupted attempt's transcript, and the manifest hashes only the last
attempt. Retrying costs the evidence of the attempt that was interrupted, unless
an attempt gets its own place in the directory.

**And the job may have already passed.** Measured above: a job the index calls
`interrupted` can hold a complete manifest whose gate passed, and `haas verify`
agrees with it. `index.go:40-46` marks a job interrupted from `state.json` alone.
Retrying such a job spends an agent run on work that is finished, and overwrites
the evidence that proves it. Before retrying is a question worth answering,
`Open` has to read `out/manifest.json`: a job with a manifest ended, and its
state is the gate's answer, not `interrupted`.

### The memory cap

`sandbox.Run` sets `--memory`, `HAAS_JOB_MEMORY` or `3g`
(`sandbox.go:45-51`). Which state a capped job ends in depends on which process
the kernel picks, and the two answers are far apart.

| the OOM killer picks | measured | the job ends |
| --- | --- | --- |
| the harness, a child in the box | container exit follows the gate, `OOMKilled` false | `passed` or `failed`, by the gate |
| the runner, PID 1 in the box | container exit 137, `OOMKilled` true | `failed`, `exit` 137 |

Both rows are from a run at `--memory=32m`: a container whose PID 1 allocates
without bound exits 137 with `OOMKilled=true`, and one whose child allocates
while PID 1 waits exits with PID 1's own code and `OOMKilled=false`.

In this product PID 1 is `haas inside`, a small Go binary, and the harness is a
much larger child. The first row is the likely one. So the common answer is that
a job whose harness is killed by the cap ends `passed` or `failed` like any other
job, decided by a gate run over a half-edited workspace.

Can a reader tell that apart from a harness that simply failed? Partly.

| in the evidence | a harness that failed | a harness killed by the cap |
| --- | --- | --- |
| `harnessExit` | its own code, 0 or above | -1 |
| `stoppedAtBudget` | false | false |
| the job's state | the gate's answer | the gate's answer |

So `harnessExit: -1` with `stoppedAtBudget: false` says a signal killed the
harness, which a harness exiting on its own never shows. It does not say which
signal, and it does not say the cap: SIGKILL from the cap, a segfault, and a
SIGTERM from anywhere else are all -1 (`inside.go:139-147`). Nothing reads the
cap, and `--rm` takes the container's `OOMKilled` flag away with the container
(`sandbox.go:45`). The manifest has no field for the limits the job ran under.

Not decided: whether the manifest records the caps a job ran under and the signal
that ended the harness, or whether a run that hit the memory cap gets a state of
its own rather than a gate verdict over a workspace the agent did not finish
editing.

### Two servers on one jobs directory

Two servers pointed at the same jobs directory both claim the same job, and
nothing prevents it. `Index` is one process's map behind one mutex, and the
comment says so: a second runner is what would earn a database
(`index.go:12-14`).

Measured on two indexes over one directory:

| what happens | result |
| --- | --- |
| both call `Claim` on one queued job | both get it, the same ID |
| a second server starts while the first runs a job | the second writes `interrupted` over the first's `running` job |
| the first server then calls `Finish` | the disk says `passed`, and the second server's memory still says `interrupted` |

The second and third rows are worse than the double claim. A server starting up
marks every `running` job in the directory as interrupted, whether or not another
server is running it (`index.go:40-46`), and a watcher on that server is sent
`event: ended` with `interrupted` for a job that is still going.

What would prevent it: the jobs directory needs one exclusive lock, taken at
`Open` and held for the life of the process, so a second server on the same
directory refuses to start and says which process holds it. A lock the kernel
releases when the holder dies also gives `Open` the thing it is missing — the
difference between "nobody holds this job" and "another server holds it" — which
is what makes the interrupted sweep correct rather than a guess. Claiming a job
with an atomic create per job would make two servers safe instead of refusing the
second, and that is a larger decision: it is the first piece of a fleet, and the
queue is one process today.

### Four faults in the code

Each of these is a place where the code does not do what the code around it says,
or cannot run at all. None is fixed here.

**`Queue.Stop` drains the queue instead of stopping it.** The worker loop runs
`for q.one() {}` and only then looks at the stop channel
(`internal/jobs/queue.go:66`). `one` returns true whenever it claimed and ran a
job, so a worker finishing the job in flight claims the next queued one.
Measured: `Stop` called with one job in flight and five more queued ran all six,
and returned with nothing left queued. So a stop meant to let the job in flight
finish starts fresh jobs inside the 60 seconds before systemd kills the process,
and whichever one is running when the kill arrives ends `interrupted`.

**The five-second tick cannot do what it says.** The comment says the tick picks
up a job queued by the command line in the same directory without anybody telling
the server (`queue.go:61-63`). Nothing re-reads the directory after `Open`.
`Claim` iterates the in-memory map (`index.go:92-102`), which only `Open` and
`Add` fill. Measured: a job directory with a `queued` `state.json`, written into
the server's jobs directory after the server started, is still `queued` 13
seconds later with an empty `out/`. `haas new` writes no `state.json` at all, so
the case the comment describes cannot happen even at boot.

**An interrupted job keeps `"exit": 0`.** `Open` sets the state and `endedAt` and
leaves `Exit` alone (`index.go:40-46`), and `Exit` has no `omitempty`
(`state.go:43`). Measured on an interrupted job: `{"state": "interrupted",
"exit": 0}`. A reader who checks the exit code reads success.

**One branch in the watcher cannot be reached.** `ended` treats a job the index
does not know as interrupted (`internal/web/stream.go:62-67`). `watch` has
already looked the job up and returned 404 when it was missing
(`stream.go:20-23`), and nothing ever removes a record from the index — there is
no `delete` on `Index.records` anywhere. The branch is unreachable, which makes
it the wrong place to decide what a missing job means.
