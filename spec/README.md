# Specs

TLA+ specifications of gusto's state machines that races and crashes can break. TLC checks each one on a small model.

```bash
spec/check.sh    # fetches tla2tools.jar v1.7.4 once; needs Java 11 or later
```

Expected output:

```
ok   TaskLifecycle.before-recovery.cfg: Error: Invariant PassedIsEarned is violated.
ok   TaskLifecycle.before-stop-race.cfg: Error: Invariant StopWins is violated.
ok   TaskLifecycle.now.cfg: 1060 states generated, 577 distinct states found, 0 states left on queue.
```

A `*.before-*.cfg` config models the code before a fix and must fail. It shows that the spec finds the bug the fix closed.
Every other config must pass.

## TaskLifecycle

One task in `internal/jobs`: the queue's worker (`Queue.one`), a person stopping it (`Queue.Cancel`), the run, and a
server crash and restart (`Open`). Each action names the Go function it models.

| Property | What it says | Before getlatentic/gusto#559 | Go test |
| --- | --- | --- | --- |
| `StopWins` | A stop the person was told took always ends the task Interrupted. | Broken: the stop lands after the worker looked | `TestAStopThatComesAsTheWorkReturnsStillEndsTheTaskInterrupted` |
| `PassedIsEarned` | A task passes only on a manifest that earned it. | Broken: recovery passed a turn whose harness failed | `TestARunThatEndedWhileNobodyWatchedIsGradedAsIfWatched` |
| `GradedAsEarned` | Live and recovered runs are graded by `evidence.Earned`. | | |
| `EndsOnce` | A finished task never changes state. | | |
| `EveryTaskEnds` | Every task finishes, with up to one crash. | | |

The model assumes `state.json` is written whole. Torn writes are getlatentic/planning#769.

## Changing a modelled machine

1. Change the spec's action to match the new Go code.
2. Run `spec/check.sh`.
3. When TLC finds a violation, write a Go test that replays the trace with explicit steps, see it fail, then fix the code.
