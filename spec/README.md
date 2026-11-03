# Specs

TLA+ specifications of gusto's state machines that races and crashes can break. TLC checks each one on a small model.

```bash
spec/check.sh    # fetches tla2tools.jar v1.7.4 once; needs Java 11 or later
```

Expected output:

```
ok   ConnectorSignIn.before-563.cfg: Error: Invariant DisconnectHolds is violated.
ok   ConnectorSignIn.before-broken.cfg: Error: Temporal properties were violated.
ok   ConnectorSignIn.before-finish.cfg: Error: Invariant DisconnectHolds is violated.
ok   ConnectorSignIn.now.cfg: 14766 states generated, 5237 distinct states found, 0 states left on queue.
ok   OrgSupervisor.before-560.cfg: Error: Invariant EndingStopsServer is violated.
ok   OrgSupervisor.before-provision.cfg: Error: Invariant EndingStopsServer is violated.
ok   OrgSupervisor.now.cfg: 1280 states generated, 411 distinct states found, 0 states left on queue.
ok   StateFile.before-dir-sync.cfg: Error: Invariant NothingDoneIsLost is violated.
ok   StateFile.before-in-place.cfg: Error: Invariant AlwaysWhole is violated.
ok   StateFile.before-unsynced.cfg: Error: Invariant AlwaysWhole is violated.
ok   StateFile.now.cfg: 771 states generated, 318 distinct states found, 0 states left on queue.
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
guard. The model leaves out two front doors at once, which is getlatentic/planning#775.

## ConnectorSignIn

One person's sign-in to one app (`internal/connector`): `Finish`, `Token` renewing, `Disconnect`, the app revoking the
grant, and the server restarting. The app rotates refresh tokens, and a vault write may fail. `Fixes` says which fixes
the code has.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `DisconnectHolds` | Nothing begun before a disconnect is kept after it. | `disconnectLocks` (getlatentic/gusto#563): a renewal writes the tokens back; `finishChecksGen`: a sign-in that came back before the disconnect is kept | `TestDisconnectingDuringARenewalStaysDisconnected`, `TestDisconnectingWhileASignInFinishesStaysDisconnected` |
| `DeadSignInIsSeen` | A sign-in the app no longer renews stops showing as connected. | `refusedMarksBroken`: it shows as connected for good, and agents are told nothing is disconnected | `TestASignInTheAppNoLongerTakesIsNotConnected` |

`DeadSignInIsSeen` assumes the app answers and the disk takes a write in the end (strong fairness), and that somebody
keeps using the app.

## StateFile

One state file that gusto replaces with each new version, as a task's `state.json` or a room's thread file, through a
process crash or a host crash. The file system has two layers: what processes see, and what a host crash leaves on
disk. The kernel writes a file's data and a directory's names to disk on its own, in either order.

| Property | What it says | Broken by | Go test |
| --- | --- | --- | --- |
| `AlwaysWhole` | A reader finds a whole version, never an empty or torn file. | writing in place (`os.WriteFile`): a reader, or a crash, finds it half written. Renaming without fsyncing the file first: a host crash keeps the rename and not the data, and the file is empty | `TestAReaderNeverFindsHalfAFile` |
| `NothingDoneIsLost` | A version said to be written is still there after a crash. | renaming without fsyncing the directory: a host crash forgets the rename | the model only: a test cannot crash the host |

`atomicfile.WriteFile` is the protocol that passes: write a temporary file, fsync it, rename it over the file, fsync
the directory.

## Changing a modelled machine

1. Change the spec's action to match the new Go code.
2. Run `spec/check.sh`.
3. When TLC finds a violation, write a Go test that replays the trace with explicit steps, see it fail, then fix the code.
