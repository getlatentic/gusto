# Specs

TLA+ specifications of gusto's state machines that races and crashes can break. TLC checks each one on a small model.

```bash
spec/check.sh    # fetches tla2tools.jar v1.7.4 once; needs Java 11 or later
```

Expected output:

```
ok   FrontDoors.before-leader.cfg: Error: Invariant OneServer is violated.
ok   FrontDoors.now.cfg: 55269 states generated, 11534 distinct states found, 0 states left on queue.
ok   OrgSupervisor.before-560.cfg: Error: Invariant EndingStopsServer is violated.
ok   OrgSupervisor.before-provision.cfg: Error: Invariant EndingStopsServer is violated.
ok   OrgSupervisor.now.cfg: 1280 states generated, 411 distinct states found, 0 states left on queue.
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

## OrgSupervisor

One organization address in the front door's controller (`internal/cloud/controller.go`), across the organizations that
may hold it one after another: a demo's address has two random bytes, so it can be given again. The callers of
`provision` are the signup's goroutine, `Run`'s goroutines at start, and the supervisor. The demo sweep removes with
`Controller.Remove`, then `Store.EndDemo`, then `Controller.Ended`. `Fixes` says which fixes the code has.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `EndingStopsServer` | While a demo is being ended, nothing starts its server. | either fix: a `provision` decided before `Remove` runs after it | `TestAProvisionThatRunsAfterRemoveIsRefused`, `TestARemovedOrganizationIsNotStartedAgain` |
| `NothingOutlivesOrg` | The directory, runtime row and server exist only while the organization does. | `provisionChecksOrg` | `TestAnOrganizationGoneFromTheStoreIsNotProvisioned` |
| `NewOrgIsProvisioned` | A new organization at a removed address is provisioned. | a check that ignores when the organization was made | `TestANewOrganizationAtARemovedAddressIsProvisioned` |

`provisionChecksOrg` alone keeps `EndingStopsServer`: the supervisor's own check from getlatentic/gusto#560 is a second
guard. FrontDoors models several front doors.

## FrontDoors

Two front doors on one database, each with a controller over one organization, and two cells. With `leader`, one front
door at a time holds a lease with an epoch, only it acts, and a runtime write is refused unless its epoch is the lease's.
The lease may lapse while its holder is paused, and the holder acts on until it notices.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `OneServer` | The organization's server runs on one cell at most. | `leader`: each front door places it on a cell of its own and starts it there. With the lease but no epoch on writes, a paused leader does the same | `TestAControllerThatLostTheLeaseStartsNothing`, `TestLeadershipRunsTheWorkOnOneFrontDoorAtATime` |

## Changing a modelled machine

1. Change the spec's action to match the new Go code.
2. Run `spec/check.sh`.
3. When TLC finds a violation, write a Go test that replays the trace with explicit steps, see it fail, then fix the code.
