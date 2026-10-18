# A job's states

A job's state is a file. `state.json` sits in the job directory beside the evidence, so the directory is the whole story of the job. The in-memory index is a cache of that file, rebuilt by reading the directory (`internal/jobs/state.go`). Four writers write the file: `Open`, `Add`, `Claim` and `Finish`, all in `internal/jobs/index.go`. Nothing else writes it.

The evidence outranks the state file. A container is a child of the container runtime, not of the server, so a job whose server was killed often runs on and finishes. `Open` reads `out/manifest.json` before it decides anything about a job the file calls `running`.

## The states

| State | Meaning | Written by |
| --- | --- | --- |
| `queued` | The job is on disk and nobody has taken it | `Add` |
| `running` | A worker holds it | `Claim` |
| `passed` | The gate exited 0 | `Finish`, or `Open` from the manifest |
| `failed` | The gate exited non-zero, or the job could not run | `Finish`, or `Open` from the manifest |
| `interrupted` | The run left no manifest, so nothing is known about it. A stopped job also ends here (see Stopping a job) | `Open` |

`passed`, `failed` and `interrupted` are final. Nothing reads a final state and queues the job again.

- A manifest counts as this run's only when it was written after the run started. Claiming a job removes the manifest its last run left.
- A recovered job takes its end from the manifest's `atUnix`, not from the clock at recovery. `endedAt` is when a server wrote the state, so it can differ from the run's end by the length of an outage.
- A directory with no `state.json` is not a job the index knows, and `Open` skips it. A job made with `gusto new` inside the server's jobs directory is invisible to the server.
- `instance` names the instance a job ran on. A task or a turn made on the pages records it. A job sent to `POST /api/jobs` has none, and the pages list and open it as running on no instance.

## The transitions

| From | To | Cause | The file gains |
| --- | --- | --- | --- |
| none | `queued` | A task on an instance (`internal/web/tasks.go`), a turn in a thread (`internal/web/instances.go`) or `POST /api/jobs` (`internal/web/api.go`) is accepted | Every field and `queuedAt`. `instance` for a task or turn. `thread` for a turn |
| `queued` | `running` | A worker takes the oldest queued job under the index mutex, so two workers in one process cannot take the same job | `startedAt` |
| `running` | `passed` | The work returns exit 0 (`internal/jobs/queue.go`) | `exit` 0, `endedAt` |
| `running` | `failed` | The work returns a non-zero exit, or an error | That `exit`, or -1 for an error, and `endedAt` |
| `running` | `passed` or `failed` | A server starts and the manifest says the gate passed or failed | `exit` from `manifest.gate.code`, `endedAt` |
| `running` | `interrupted` | A server starts and there is no manifest | `exit` -1, `endedAt` |

## What reaches the disk while a job runs

`Claim` writes `state.json` before the work starts. The runner inside the box writes the rest, in this order (`internal/runner/inside.go`):

| Written | When |
| --- | --- |
| `out/harness.log`, `out/transcript.log`, `out/calls.jsonl`, `out/stderr.log` | Before the harness starts, truncated. A line is added as the harness prints it |
| `out/diff.patch` | After the harness stops |
| `out/gate.log`, `out/gates.json` | After the gate answers |
| `out/manifest.json` | Last, hashing everything above |

Each line reaches the bind-mounted directory before the next is read, because a container's own disk does not survive the container. `Open` reads only the manifest, whose presence separates a run that reached its end from one that did not.

The runner refuses a job before the harness starts when the harness's program is not on its PATH, a restricted job names a model that is not local, or a harness needing a local model has none answering (`internal/runner/admit.go`). A refusal writes only `out/transcript.log`, holding the reason. It leaves no gate verdict and no manifest. In the box the refusal reaches the queue as exit 2. In the server's own process it reaches the queue as an error.

The time budget ends a harness. It comes from `budget.seconds`, default 900 (`internal/job/job.go`). `budget.turns` defaults to 40 and nothing reads it. A money cap does not end the harness. The proxy refuses the job's next model call, and the time budget ends a harness that carries on without a model. The runner reads the refusal from `out/proxy.jsonl` and records `stoppedByCap` in the manifest (`internal/runner/capped.go`). The job's state is still the gate's answer.

## A server restart

| How the server stops | What happens to the job in flight |
| --- | --- |
| SIGINT or SIGTERM | The job runs on, and `Finish` records it if it ends within 60 seconds |
| SIGKILL, a crash, the machine | The container runs on, and the next start reads its manifest |

- On a signal the server shuts the HTTP server down with a 30-second grace. `queue.Stop()` then waits for every worker. A worker checks for stop between jobs, so nothing new starts (`cmd/gusto/serve.go`, `internal/jobs/queue.go`).
- The job in flight is not interrupted, so the stop takes as long as that job. The service unit allows 60 seconds, then kills everything in the cgroup (`deploy/ansible/roles/serve/templates/gusto.service.j2`). A job with the default 900-second budget often meets the hard kill. Jobs left queued are the next server's work.
- A job left `queued` stays `queued`, and the next server takes it. A job that appears in the directory while a server runs waits for the next start, because the index is read once, at `Open`.
- `GUSTO_SANDBOX=here` runs the work as a function call, so a job dies with the server.

After a hard kill the file still says `running`. The next start decides from the evidence:

| The container | On the next start |
| --- | --- |
| Reached the gate and wrote its manifest | `passed` or `failed`, from `manifest.gate` |
| Died first, or was killed with the server | `interrupted`, `exit` -1 |

## A runner that dies mid-job

| What dies | Harness line in the manifest | The job's state |
| --- | --- | --- |
| The harness, on its own non-zero exit or by a signal | `harnessExit` is that code, or -1 for a signal | The gate decides |
| The harness, at the time budget | `harnessExit` -1, `stoppedAtBudget` true | The gate decides |
| The harness, because a person stopped the job | `harnessExit` -1, `stoppedBy` the person (`a signal` when nobody is named) | `interrupted`, whatever the gate would say |
| The harness, with its model refused at a spending cap | Whatever it exits with, and `stoppedByCap` | The gate decides |
| A process the harness started | `harnessExit` is what the harness reports for it | The gate decides |
| The runner, PID 1 in the box | No manifest | `failed`, with the container's exit |
| The container never starts | No manifest, `out/` empty | `failed`, `exit` 125 |
| The runner, on an error before the gate | No manifest | `failed`, `exit` 2 in the box, -1 on the host |
| `gusto serve` | Written by the container that outlives it | The manifest's verdict, or `interrupted` |

- A killed harness does not fail a job. The gate decides, on whatever the harness left in the workspace.
- `harnessExit` is what the process the runner started reported, and nothing deeper. A harness that is a shell reports its child's kill as its own status, such as 137.
- `stoppedAtBudget` and `stoppedBy` are the fields that name a cause.
- Inside the box, `gusto inside` turns an error into exit 2 and the queue reads a non-zero exit (`cmd/gusto/main.go`). On the host (reading `job.json`, cloning, starting the container) the error reaches the queue as an error and becomes `exit` -1.

## What a watcher sees

`/watch/{id}` tails `out/transcript.log` every 300 milliseconds and asks the index whether the job has ended (`internal/web/stream.go`). A late watcher is caught up from the beginning. An unknown job is answered 404 before a stream opens.

| The job is | The watcher gets |
| --- | --- |
| `queued` | An open stream and no data, so the page reads `waiting…` |
| `running` | Each line, in the order the runner wrote it |
| `passed`, `failed` | One last read of the file, then `event: ended` with the state |
| `interrupted` | The transcript up to the kill, then `event: ended` with `interrupted` |

A watcher is not told about a server restart. The cookie key is made at boot, so every old cookie is invalid and the reconnect is answered with 303 to `/login`. The page opens a stream only when it was rendered `queued` or `running` (`internal/web/templates/job.html`). Reload and sign in, and the page shows what the evidence decided.

## Stopping a job

A person stops a turn of their own, and an admin stops anybody's, with the Stop button on the conversation or `POST /api/jobs/{id}/stop` with a person's token.

- A queued job is interrupted at once and never runs. Its record has `stoppedBy`.
- A running job is told to stop. The server writes `stop.json` in the job's directory (who, and when) and interrupts the container's client. `gusto inside` then kills the harness and everything the harness started, keeps what it did in the workspace, and writes the manifest with `stoppedBy`. The server signs it. The job ends `interrupted`, not `failed`, because the gate's answer over half a piece of work is not a verdict on the agent.
- A container still there after sixty seconds is removed by its name, `gusto-<job id>`.
- A job that is over answers 409 and is not changed.
- A run stopped by a signal alone, as when a server shuts down, has `stoppedBy: "a signal"`.
- This does not stop what the agent started in the workspace and left running after its turn, such as an app it served.

## Open points

- **No retry.** Nothing retries an `interrupted` job. A safe retry needs three decisions. `sandbox.Run` passes `--rm`, so a retry is a new container. `Fill` leaves an existing `.git` as it is (`internal/runner/workspace.go`), so a retry would run over the dead attempt's edits, and the job must say whether to reset or clone again. `Open` truncates the four log files (`internal/runner/record.go`), so a retry loses the dead attempt's transcript.
- **A stale manifest is trusted.** Nothing clears `out/manifest.json` before a run starts. Retrying a job that already has a verdict is unsafe until a run removes the manifest it will replace.
- **The memory cap.** `sandbox.Run` sets `--memory` from `GUSTO_JOB_MEMORY`, default `3g` (`internal/sandbox/sandbox.go`). When the OOM killer picks the harness, the gate decides. When it picks the runner, the job ends `failed` with `exit` 137. `harnessExit` -1 shows a signal killed the harness, but not which one. The manifest has no field for the limits a job ran under or the signal that ended the harness. A cap the runtime refuses, such as `GUSTO_JOB_MEMORY=4m`, ends `failed` with `exit` 125 and `ready` does not catch it.
- **Two servers on one jobs directory.** Both claim the same job, and a server starting up writes `interrupted` over a `running` job the other server is running. A single exclusive lock on the jobs directory would prevent it. `Index` is one process's map behind one mutex (`internal/jobs/index.go`).
