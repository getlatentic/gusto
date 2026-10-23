# A task's states

A task's state is a file. `state.json` sits in the task directory beside the record, so the directory is the whole story of the task. The in-memory index is a cache of that file, rebuilt by reading the directory (`internal/jobs/state.go`). Four writers write the file: `Open`, `Add`, `Claim` and `Finish`, all in `internal/jobs/index.go`. Nothing else writes it.

The record outranks the state file. A container is a child of the container runtime, not of the server, so a task whose server was killed often runs on and finishes. `Open` reads `out/manifest.json` before it decides anything about a task the file calls `running`.

## The states

| State | Meaning | Written by |
| --- | --- | --- |
| `queued` | The task is on disk and nobody has taken it | `Add` |
| `running` | A worker holds it | `Claim` |
| `passed` | The check exited 0 | `Finish`, or `Open` from the manifest |
| `failed` | The check exited non-zero, or the task could not run | `Finish`, or `Open` from the manifest |
| `interrupted` | The run left no manifest, so nothing is known about it. A stopped task also ends here (see Stopping a task) | `Open` |

`passed`, `failed` and `interrupted` are final. Nothing reads a final state and queues the task again.

- A manifest counts as this run's only when it was written after the run started. Claiming a task removes the manifest its last run left.
- A recovered task takes its end from the manifest's `atUnix`, not from the clock at recovery. `endedAt` is when a server wrote the state, so it can differ from the run's end by the length of an outage.
- A directory with no `state.json` is not a task the index knows, and `Open` skips it. A task made with `gusto new` inside the server's tasks directory is invisible to the server.
- `instance` names the instance a task ran on. A task or a turn made on the pages records it. A task sent to `POST /api/jobs` has none, and the pages list and open it as running on no instance.

## The transitions

| From | To | Cause | The file gains |
| --- | --- | --- | --- |
| none | `queued` | A task on an instance (`internal/web/tasks.go`), a turn in a thread (`internal/web/instances.go`) or `POST /api/jobs` (`internal/web/api.go`) is accepted | Every field and `queuedAt`. `instance` for a task or turn. `thread` for a turn |
| `queued` | `running` | A worker takes the oldest queued task under the index mutex, so two workers in one process cannot take the same task | `startedAt` |
| `running` | `passed` | The work returns exit 0 (`internal/jobs/queue.go`) | `exit` 0, `endedAt` |
| `running` | `failed` | The work returns a non-zero exit, or an error | That `exit`, or -1 for an error, and `endedAt` |
| `running` | `passed` or `failed` | A server starts and the manifest says the check passed or failed | `exit` from `manifest.gate.code`, `endedAt` |
| `running` | `interrupted` | A server starts and there is no manifest | `exit` -1, `endedAt` |

## What reaches the disk while a task runs

`Claim` writes `state.json` before the work starts. The runner inside the box writes the rest, in this order (`internal/runner/inside.go`):

| Written | When |
| --- | --- |
| `out/harness.log`, `out/transcript.log`, `out/calls.jsonl`, `out/stderr.log` | Before the agent starts, truncated. A line is added as the agent prints it |
| `out/diff.patch` | After the agent stops |
| `out/gate.log`, `out/gates.json` | After the check answers |
| `out/manifest.json` | Last, hashing everything above |

Each line reaches the bind-mounted directory before the next is read, because a container's own disk does not survive the container. `Open` reads only the manifest, whose presence separates a run that reached its end from one that did not.

The runner refuses a task before the agent starts when the agent's program is not on its PATH, a restricted task names a model that is not local, or an agent needing a local model has none answering (`internal/runner/admit.go`). A refusal writes only `out/transcript.log`, holding the reason. It leaves no check verdict and no manifest. In the box the refusal reaches the queue as exit 2. In the server's own process it reaches the queue as an error.

The time budget ends an agent. It comes from `budget.seconds`, default 900 (`pkg/job/job.go`). `budget.turns` defaults to 40 and nothing reads it. A money cap does not end the agent. The proxy refuses the task's next model call, and the time budget ends an agent that carries on without a model. The runner reads the refusal from `out/proxy.jsonl` and records `stoppedByCap` in the manifest (`internal/runner/capped.go`). The task's state is still the check's answer.

## A server restart

| How the server stops | What happens to the task in flight |
| --- | --- |
| SIGINT or SIGTERM | The task runs on, and `Finish` records it if it ends within 60 seconds |
| SIGKILL, a crash, the machine | The container runs on, and the next start reads its manifest |

- On a signal the server shuts the HTTP server down with a 30-second grace. `queue.Stop()` then waits for every worker. A worker checks for stop between tasks, so nothing new starts (`cmd/gusto/serve.go`, `internal/jobs/queue.go`).
- The task in flight is not interrupted, so the stop takes as long as that task. The service unit allows 60 seconds, then kills everything in the cgroup (`deploy/ansible/roles/serve/templates/gusto.service.j2`). A task with the default 900-second budget often meets the hard kill. Tasks left queued are the next server's work.
- A task left `queued` stays `queued`, and the next server takes it. A task that appears in the directory while a server runs waits for the next start, because the index is read once, at `Open`.
- `GUSTO_SANDBOX=here` runs the work as a function call, so a task dies with the server.

After a hard kill the file still says `running`. The next start decides from the record:

| The container | On the next start |
| --- | --- |
| Reached the check and wrote its manifest | `passed` or `failed`, from `manifest.gate` |
| Died first, or was killed with the server | `interrupted`, `exit` -1 |

## A runner that dies mid-task

| What dies | Agent line in the manifest | The task's state |
| --- | --- | --- |
| The agent, on its own non-zero exit or by a signal | `harnessExit` is that code, or -1 for a signal | The check decides |
| The agent, at the time budget | `harnessExit` -1, `stoppedAtBudget` true | The check decides |
| The agent, because a person stopped the task | `harnessExit` -1, `stoppedBy` the person (`a signal` when nobody is named) | `interrupted`, whatever the check would say |
| The agent, with its model refused at a spending cap | Whatever it exits with, and `stoppedByCap` | The check decides |
| A process the agent started | `harnessExit` is what the agent reports for it | The check decides |
| The runner, PID 1 in the box | No manifest | `failed`, with the container's exit |
| The container never starts | No manifest, `out/` empty | `failed`, `exit` 125 |
| The runner, on an error before the check | No manifest | `failed`, `exit` 2 in the box, -1 on the host |
| `gusto serve` | Written by the container that outlives it | The manifest's verdict, or `interrupted` |

- A killed agent does not fail a task. The check decides, on whatever the agent left in the workspace.
- `harnessExit` is what the process the runner started reported, and nothing deeper. An agent that is a shell reports its child's kill as its own status, such as 137.
- `stoppedAtBudget` and `stoppedBy` are the fields that name a cause.
- Inside the box, `gusto inside` turns an error into exit 2 and the queue reads a non-zero exit (`cmd/gusto/main.go`). On the host (reading `job.json`, cloning, starting the container) the error reaches the queue as an error and becomes `exit` -1.

## What a watcher sees

`/watch/{id}` tails `out/transcript.log` every 300 milliseconds and asks the index whether the task has ended (`internal/web/stream.go`). A late watcher is caught up from the beginning. An unknown task is answered 404 before a stream opens.

| The task is | The watcher gets |
| --- | --- |
| `queued` | An open stream and no data, so the page reads `waiting…` |
| `running` | Each line, in the order the runner wrote it |
| `passed`, `failed` | One last read of the file, then `event: ended` with the state |
| `interrupted` | The transcript up to the kill, then `event: ended` with `interrupted` |

A watcher is not told about a server restart. The cookie key is made at boot, so every old cookie is invalid and the reconnect is answered with 303 to `/login`. The page opens a stream only when it was rendered `queued` or `running` (`internal/web/templates/job.html`). Reload and sign in, and the page shows what the record decided.

## Stopping a task

A person stops a turn of their own, and an admin stops anybody's, with the Stop button on the conversation or `POST /api/jobs/{id}/stop` with a person's token.

- A queued task is interrupted at once and never runs. Its record has `stoppedBy`.
- A running task is told to stop. The server writes `stop.json` in the task's directory (who, and when) and interrupts the container's client. `gusto inside` then kills the agent and everything the agent started, keeps what it did in the workspace, and writes the manifest with `stoppedBy`. The server signs it. The task ends `interrupted`, not `failed`, because the check's answer over half a piece of work is not a verdict on the agent.
- A container still there after sixty seconds is removed by its name, `gusto-<task id>`.
- A task that is over answers 409 and is not changed.
- A run stopped by a signal alone, as when a server shuts down, has `stoppedBy: "a signal"`.
- This does not stop what the agent started in the workspace and left running after its turn, such as an app it served.

## Open points

- **No retry.** Nothing retries an `interrupted` task. A safe retry needs three decisions. `sandbox.Run` passes `--rm`, so a retry is a new container. `Fill` leaves an existing `.git` as it is (`internal/runner/workspace.go`), so a retry would run over the dead attempt's edits, and the task must say whether to reset or clone again. `Open` truncates the four log files (`internal/runner/record.go`), so a retry loses the dead attempt's transcript.
- **A stale manifest is trusted.** Nothing clears `out/manifest.json` before a run starts. Retrying a task that already has a verdict is unsafe until a run removes the manifest it will replace.
- **The memory cap.** `sandbox.Run` sets `--memory` from `GUSTO_JOB_MEMORY`, default `3g` (`internal/sandbox/sandbox.go`). When the OOM killer picks the agent, the check decides. When it picks the runner, the task ends `failed` with `exit` 137. `harnessExit` -1 shows a signal killed the agent, but not which one. The manifest has no field for the limits a task ran under or the signal that ended the agent. A cap the runtime refuses, such as `GUSTO_JOB_MEMORY=4m`, ends `failed` with `exit` 125 and `ready` does not catch it.
- **Two servers on one tasks directory.** Both claim the same task, and a server starting up writes `interrupted` over a `running` task the other server is running. A single exclusive lock on the tasks directory would prevent it. `Index` is one process's map behind one mutex (`internal/jobs/index.go`).
