# A job's states

A job's state is a file. `state.json` sits in the job directory beside the
evidence, so the directory is the whole story of the job and nothing is left
behind in a database. The index in memory is a cache of that file, rebuilt by
reading the directory (`internal/jobs/state.go:1-8`).

There are five states and four writers. The writers are `Open`, `Add`, `Claim`
and `Finish`, all in `internal/jobs/index.go`. Nothing else writes the file.

The evidence outranks the state file. A container is a child of the container
runtime, not of the server, so a job whose server was killed often runs on and
finishes. `Open` therefore reads `out/manifest.json` before it decides anything
about a job the file calls `running` (`index.go:22-28`, `138-155`).

## The states

| state | what it says | written by |
| --- | --- | --- |
| `queued` | the job is on disk and nobody has taken it | `Add`, `index.go:60-71` |
| `running` | a worker holds it | `Claim`, `index.go:94-118` |
| `passed` | the gate exited 0 | `Finish`, `index.go:121-136`; `ended`, `index.go:149-153` |
| `failed` | the gate exited non-zero, or the job could not run | `Finish`, `index.go:121-136`; `ended`, `index.go:149-153` |
| `interrupted` | the run left no manifest, so nothing is known about it | `ended`, `index.go:143-147` |

A manifest counts as this run's only when it was written after the run started,
and claiming a job removes the one its last run left (`index.go`). A directory can
be run again, and without both rules a second run that never reached its gate
comes back carrying the first run's verdict. A recovered job takes its end from
the manifest's `atUnix` rather than from the clock at recovery, so an outage is
not counted as part of the job.

`passed`, `failed` and `interrupted` are final. `Done` reports all three, and
nothing reads a final state and queues the job again
(`internal/jobs/state.go:29-31`).

A directory with no `state.json` is not a job the index knows, and `Open` skips
it (`index.go:42-45`). `haas new` writes `job.json` and the adapter and no
`state.json`, so a job made on the command line inside the server's jobs
directory is invisible to the server.

## The transitions

| from | to | what causes it | what the file gains |
| --- | --- | --- | --- |
| — | `queued` | the form is accepted (`internal/web/pages.go:95-103`) | every field, `queuedAt` |
| `queued` | `running` | a worker takes the oldest queued job (`index.go:94-118`) | `startedAt` |
| `running` | `passed` | the work returns exit 0 (`internal/jobs/queue.go:96`) | `exit` 0, `endedAt` |
| `running` | `failed` | the work returns a non-zero exit (`queue.go:102-104`) | that `exit`, `endedAt` |
| `running` | `failed` | the work returns an error (`queue.go:98-101`) | `exit` -1, `endedAt` |
| `running` | `passed` | a server starts and the job's manifest says the gate passed (`index.go:149-153`) | `exit` from `manifest.gate.code`, `endedAt` |
| `running` | `failed` | a server starts and the manifest says the gate failed (`index.go:149-153`) | `exit` from `manifest.gate.code`, `endedAt` |
| `running` | `interrupted` | a server starts and there is no manifest (`index.go:143-147`) | `exit` -1, `endedAt` |

`Claim` picks the queued job with the lowest `queuedAt` under the index mutex,
so two workers in one process cannot take the same job (`index.go:94-118`).

`endedAt` is when a server wrote the state, not when the run ended
(`index.go:140`). For a job recovered from its manifest the two differ by the
length of the outage; the manifest's `atUnix` says when the run ended. Measured
on a recovered job: `endedAt` 5 seconds after the manifest's `atUnix`, and the
page reporting `took 20s` for a run of 15 seconds.

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

`out/manifest.json` is the last thing written and the only thing `Open` reads, so
the manifest's presence is what separates a run that reached its end from one
that did not.

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
| SIGKILL, a crash, the machine | the container runs on, and the next start reads its manifest |

On a signal the handler shuts the HTTP server down with a 30-second grace
(`cmd/haas/serve.go:53-60`). `queue.Stop()` then closes the stop channel and
waits for every worker (`queue.go:54-57`). A worker checks that channel between
jobs, so stopping starts nothing new (`queue.go:66-75`). Measured with one worker
and six jobs queued: one job ran and five were still queued when `Stop`
returned.

The job in flight is not interrupted, so the stop takes as long as that job does.
The service unit gives the whole thing 60 seconds and then kills everything in
the cgroup (`deploy/ansible/roles/serve/templates/haas.service.j2:23-24`). A job
with the default budget may take 900 seconds, so the one job in flight often
meets the hard kill. The comment above those two lines says the service gets time
to end its jobs and write their evidence before anything is killed
(`haas.service.j2:21-22`). That holds for a job of under a minute and for no
other. The five left queued cost nothing: they are the next server's work.

After a hard kill the file still says `running`, and what the job ends as depends
on how far the container got.

| the container | on the next start |
| --- | --- |
| reached the gate and wrote its manifest | `passed` or `failed`, from `manifest.gate` |
| died first, or was killed with the server | `interrupted`, `exit` -1 |

Both rows are from a run. A server SIGKILLed 12 seconds into a harness that
prints for 15 left the container running; the container finished the harness, ran
the gate, wrote seven evidence files and a manifest saying `"passed": true`, and
the next start recorded `passed` with `exit` 0. Killing the container as well
left the four log files, no manifest, and a next start recording `interrupted`
with `exit` -1.

A job running in the server's own process dies with the server instead, because
`HAAS_SANDBOX=here` makes the work a function call rather than a container
(`cmd/haas/serve.go:70-86`).

A job left `queued` by a stopped server stays `queued`, and the next server takes
it. Measured: a job directory with a `queued` `state.json`, present before the
server starts, ends `passed`.

The index is read from disk once, at `Open`. Nothing re-reads the directory
afterwards, and `Claim` iterates the in-memory map (`index.go:98-107`), so a job
appearing in the directory while a server runs waits for the next start.

## A runner that dies mid-job

| what dies | the harness line in the manifest | the job's state |
| --- | --- | --- |
| the harness, on its own non-zero exit | `harnessExit` is that code | the gate decides |
| the harness, by a signal | `harnessExit` -1 | the gate decides |
| the harness, at the time budget | `harnessExit` -1, `stoppedAtBudget` true | the gate decides |
| a process the harness started | `harnessExit` is what the harness reports for it | the gate decides |
| the runner, PID 1 in the box | no manifest | `failed`, with the container's exit |
| the container never starts | no manifest, and `out/` empty | `failed`, `exit` 125 |
| the runner, on an error before the gate | no manifest | `failed`, `exit` 2 in the box, -1 on the host |
| `haas serve` | written by the container that outlives it | the manifest's verdict, or `interrupted` |

Measured, with a harness that kills itself and a gate that passes: the manifest
says `harnessExit: -1`, `stoppedAtBudget: false`, `gate: {code: 0, passed:
true}`, and `haas inside` exits 0. A killed harness does not fail a job. The
gate decides, on whatever the harness left in the workspace.

`harnessExit` is what the process the runner started reported, and nothing
deeper. Measured under the same memory cap: a harness that allocates without
bound itself leaves `harnessExit: -1`, and a harness that is a shell whose child
allocates leaves `harnessExit: 137`, because the shell reports its child's kill
as its own status. `stoppedAtBudget` is the only field that names a cause
(`inside.go:84`).

The last runner row has two answers because the boundary is in the middle of it.
Inside the box, `haas inside` turns an error into exit 2
(`cmd/haas/main.go:122-132`, `main.go:228-231`), the container carries that out,
and the queue reads it as a non-zero exit (`queue.go:102-104`). On the host —
reading `job.json`, cloning the repository, starting the container — the error
reaches the queue as an error and becomes `exit` -1 (`queue.go:98-101`). Measured
on the container path: a job naming a harness the image does not carry ends
`failed` with `exit` 2, and a job whose repository does not exist ends `failed`
with `exit` -1.

## What a watcher sees

`/watch/{id}` tails `out/transcript.log` every 300 milliseconds and asks the
index whether the job has ended (`internal/web/stream.go:19-58`). The file is the
stream, so a watcher who arrives late is caught up from the beginning.

A job the index does not know is answered 404 before a stream opens
(`stream.go:20-25`), and nothing removes a job from the index once it is there.
Each read takes the index's newest record of the job, and keeps the record the
watcher already holds if the index has none (`stream.go:132-141`), so the watcher
never names a verdict the index did not give.

| the job is | the watcher gets |
| --- | --- |
| `queued` | an open stream and no data, so the page reads `waiting…` |
| `running` | each line, in the order the runner wrote it |
| `passed`, `failed` | one last read of the file, then `event: ended` with the state |
| `interrupted` | the transcript up to the kill, then `event: ended` with `interrupted` |

Measured on a job whose server and container were both killed: the stream
replays the transcript and then sends `event: ended` / `data: interrupted`.

A watcher across a restart is not told. The cookie key is made at boot
(`internal/web/auth.go:24-32`), so every cookie from the old process is invalid
and the reconnect is answered with 303 to `/login` — measured. The page opens a
stream only when the state it was rendered with is `queued` or `running`, and it
listens for `ended` and for messages and not for errors
(`internal/web/templates/job.html:53-72`). The person reloads, signs in, and the
page then says what the evidence decided.

## What is not decided yet

### An interrupted job is never retried

Nothing retries it, and `interrupted` is final (`state.go:29-31`). The state now
means something narrow: the run left no manifest, so it did not reach its gate.
That is the only kind of job for which a retry is even the right question.

Three things are true of such a job, and each is a decision nobody has made.

**The container is gone.** `sandbox.Run` passes `--rm`, so the box a job ran in
does not survive it (`internal/sandbox/sandbox.go:45`). A retry is a new
container, and nothing is carried over.

**The workspace holds the dead attempt's edits.** `Fill` leaves a directory that
already holds `.git` exactly as it is (`internal/runner/workspace.go:14-17`). A
retry runs the harness over those edits, against a brief written for the original
tree. A safe retry resets the workspace to a named commit first, or clones again
— and the job has to say which, because "keep what the agent did and carry on"
and "start over" are different products.

**The evidence of the dead attempt is overwritten.** `Open` creates the four log
files with `os.Create`, which truncates (`internal/runner/record.go:41`). A retry
loses the interrupted attempt's transcript, and the manifest hashes only the last
attempt. An attempt needs its own place in the directory before that is
acceptable.

One thing stands in the way of retrying anything else. Nothing clears
`out/manifest.json` before a run starts, and `Open` trusts whatever manifest it
finds. Measured end to end: a job directory whose first run passed, queued again
with a harness that writes nothing and killed mid-run, comes back `passed` with
`exit` 0 while its workspace holds the answer its own gate rejects. So retrying a
job that already has a verdict is unsafe until a run removes the manifest it is
going to replace. Retrying an interrupted job is not exposed to this, because an
interrupted job has no manifest — but the first retry that itself dies brings the
question back.

### The memory cap

`sandbox.Run` sets `--memory`, `HAAS_JOB_MEMORY` or `3g`
(`sandbox.go:45-49, 103-109`). Which state a capped job ends in depends on which
process the kernel picks.

| the OOM killer picks | measured | the job ends |
| --- | --- | --- |
| the harness, a child in the box | the container exits with the gate's answer | `passed` or `failed`, by the gate |
| the runner, PID 1 in the box | the container exits 137 | `failed`, `exit` 137 |

The container rows are from a run at `--memory=32m`: a container whose PID 1
allocates without bound exits 137, and one whose child allocates while PID 1
waits exits with PID 1's own code, 0. The daemon's `OOMKilled` flag reported the
child's kill in one run of four, so it is not an answer even before `--rm`
removes the container that carries it.

In this product PID 1 is `haas inside`, a small Go binary, and the harness is a
much larger child, so the first row is the likely one. Measured through the
server at `--memory=64m` with a harness that allocates without bound: the run
reaches the gate, writes a manifest, and the job ends `failed` with `exit` 1 —
the gate's answer over a workspace the harness did not finish editing.

Can a reader tell that apart from a harness that simply failed? Partly.

| in the evidence | a harness that failed | a harness killed by the cap |
| --- | --- | --- |
| `harnessExit` | its own code, 0 or above | -1, or 137 through a shell |
| `stoppedAtBudget` | false | false |
| the job's state | the gate's answer | the gate's answer |

`harnessExit: -1` says a signal killed the process the runner started, which a
harness exiting on its own never shows. It does not say which signal: SIGKILL
from the cap, a segfault and a SIGTERM from anywhere else are all -1
(`inside.go:139-147`). A harness that wraps its work in a shell reports 137
instead, which is indistinguishable from a harness choosing to exit 137. Nothing
reads the cap back, and the manifest has no field for the limits the job ran
under.

The manifest-first rule does not change this, and it changes one thing next to
it. A capped harness still reaches the gate, so its verdict survives a server
restart along with it. A cap that kills the runner leaves no manifest, so a job
whose server also died collapses to `interrupted` with `exit` -1, and the 137 a
live server would have recorded is gone. The same holds for a cap the runtime
refuses outright: measured at `HAAS_JOB_MEMORY=4m`, docker answers `Minimum
memory limit allowed is 6MB`, the container never starts, `out/` stays empty and
the job ends `failed` with `exit` 125. `ready` starts a container before the real
run but passes no memory flags (`sandbox.go:81-96`), so it does not catch a cap
the runtime will refuse.

Not decided: whether the manifest records the caps a job ran under and the signal
that ended the harness, and whether an evidence-less death keeps the container's
exit code instead of -1.

### Two servers on one jobs directory

Two servers pointed at the same jobs directory both claim the same job, and
nothing prevents it. `Index` is one process's map behind one mutex, and the
comment says so: a second runner is what would earn a database
(`index.go:13-15`).

Measured on two indexes over one directory:

| what happens | result |
| --- | --- |
| both call `Claim` on one queued job | both get it, the same ID |
| a second server starts while the first runs a job | the second writes `interrupted`, `exit` -1, over the first's `running` job |
| the first server then calls `Finish` | the disk says `passed`, and the second server's memory still says `interrupted` |

The second and third rows are worse than the double claim. A server starting up
decides every `running` job in the directory, whether or not another server is
running it, and a job that has not reached its gate has no manifest to save it
(`index.go:46-51`). A watcher on that server is sent `event: ended` with
`interrupted` for a job that is still going.

What would prevent it: the jobs directory needs one exclusive lock, taken at
`Open` and held for the life of the process, so a second server on the same
directory refuses to start and says which process holds it. A lock the kernel
releases when the holder dies also gives `Open` the thing it is missing — the
difference between "nobody holds this job" and "another server holds it" — which
is what makes the sweep over `running` jobs correct rather than a guess. Claiming
a job with an atomic create per job would make two servers safe instead of
refusing the second, and that is a larger decision: it is the first piece of a
fleet, and the queue is one process today.
