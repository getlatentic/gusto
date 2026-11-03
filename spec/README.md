# Specs

TLA+ specifications of gusto's state machines that races and crashes can break. TLC checks each one on a small model.

```bash
spec/check.sh    # fetches tla2tools.jar v1.7.4 once; needs Java 11 or later
```

Expected output:

```
ok   AppendLog.before-repair.cfg: Error: Invariant AlwaysStarts is violated.
ok   AppendLog.before-rollback.cfg: Error: Invariant AlwaysStarts is violated.
ok   AppendLog.now.cfg: 515 states generated, 190 distinct states found, 0 states left on queue.
ok   Computer.before-creating.cfg: Error: Invariant NoIdleCreating is violated.
ok   Computer.before-in-use.cfg: Error: Invariant NoStopMidTask is violated.
ok   Computer.before-retire.cfg: Error: Invariant OnlyAskedIsRemoved is violated.
ok   Computer.now.cfg: 4448 states generated, 1164 distinct states found, 0 states left on queue.
ok   ConnectorSignIn.before-563.cfg: Error: Invariant DisconnectHolds is violated.
ok   ConnectorSignIn.before-broken.cfg: Error: Temporal properties were violated.
ok   ConnectorSignIn.before-finish.cfg: Error: Invariant DisconnectHolds is violated.
ok   ConnectorSignIn.now.cfg: 14766 states generated, 5237 distinct states found, 0 states left on queue.
ok   FrontDoors.before-leader.cfg: Error: Invariant OneServer is violated.
ok   FrontDoors.now.cfg: 55269 states generated, 11534 distinct states found, 0 states left on queue.
ok   MergeJournal.before-keep.cfg: Error: Invariant AllOrNothing is violated.
ok   MergeJournal.before-twice.cfg: Error: Invariant AllOrNothing is violated.
ok   MergeJournal.now.cfg: 76 states generated, 50 distinct states found, 0 states left on queue.
ok   OrgSupervisor.before-560.cfg: Error: Invariant EndingStopsServer is violated.
ok   OrgSupervisor.before-provision.cfg: Error: Invariant EndingStopsServer is violated.
ok   OrgSupervisor.now.cfg: 1280 states generated, 411 distinct states found, 0 states left on queue.
ok   RoomGuard.before-hold.cfg: Error: Invariant Exclusive is violated.
ok   RoomGuard.now.cfg: 162 states generated, 97 distinct states found, 0 states left on queue.
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
guard. FrontDoors models several front doors.

## FrontDoors

Two front doors on one database, each with a controller over one organization, and two cells. With `leader`, one front
door at a time holds a lease with an epoch, only it acts, and a runtime write is refused unless its epoch is the lease's.
The lease may lapse while its holder is paused, and the holder acts on until it notices.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `OneServer` | The organization's server runs on one cell at most. | `leader`: each front door places it on a cell of its own and starts it there. With the lease but no epoch on writes, a paused leader does the same | `TestAControllerThatLostTheLeaseStartsNothing`, `TestLeadershipRunsTheWorkOnOneFrontDoorAtATime` |

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

## AppendLog

The change log (`internal/changelog`): numbered, hash-chained lines that `Append` writes one at a time, each synced
before it returns, and that `Open` checks before the server starts. A write may be cut short by a full disk, a sync may
fail, the kernel may write part of the last line, and either the process or the host may crash.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `AlwaysStarts` | A crash or a failed write never stops the server starting. | `repairTail`: a crash leaves part of a line, and `Open` refuses the log for good. `rollback`: the next event is written after part of the last one, which no repair can take out | `TestALogEndingInPartOfALineOpensWithoutIt`, `TestAWriteCutShortLeavesNothingBehind`, `TestALogThatCannotBeCutBackTakesNoMoreEvents` |
| `NothingDoneIsLost` | Every event `Append` said was written is in the log at its number. | | |
| `RepairHidesNothing` | `Open` removes only a last line without its newline, never a whole line, so a cut or changed log is still refused. | | the existing tamper tests |

## Computer

One person's computer (`internal/computer`, `cmd/gusto/computers.go`): its record, its sandbox, the tasks that run in
it, the front door asking for it to be removed when the person is, and the organization server's tend loop, which
removes what was asked and stops what is idle. A computer made again after one was removed is a new generation under
the same id. Tasks are paid for by the owner and may be asked by a colleague, as a room agent's turn is.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `OnlyAskedIsRemoved` | A computer is removed only when its person was removed while it lived. | `retireOnlyLive`: removing a person whose computer was already removed leaves a mark, and the tend loop removes the computer made when they come back | `TestAMarkOnARemovedComputerDoesNotRemoveTheNextOne` |
| `NoStopMidTask` | No sandbox is stopped while a task works in it. | `inUse`: the idle stop asked whether the owner had a task running, so a colleague's turn in the owner's computer was stopped, and a task could start between the look and the stop | `TestTheIdleStopLeavesAComputerATaskIsIn` |
| `NoIdleCreating` | A computer is Creating only while a task is making it. | `failBeforeStart`: a task that failed before its sandbox started left the computer being made, counting against the quota | `TestAComputerThatNeverStartedIsNotLeftBeingMade` |

## RoomGuard

One room's folder in the room-work hub (`internal/roomwork`): turns join, merge under the room's guard, and leave, and a
person's upload or edit holds the guard while it writes (`Hub.Hold`). The hub keeps a state for the room, with its
guard, and drops it when it is not needed.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `Exclusive` | A merge never changes the folder while a person's change writes to it. | `holdKeepsState`: a change made with no turn in the room held nothing, and a state dropped when its last turn left gave the next turn a guard of its own | `TestAChangeHoldsOffATurnThatJoinsWhileItIsMade`, `TestAChangeHoldsTheRoomWhenItsLastTurnLeaves` |

## MergeJournal

One merge into a room's folder (`internal/roomwork/apply.go`): stage, journal, keep and replace each file, remove the
journal. A crash leaves the journal, and `recoverMerge` undoes what ran; putting a file back may fail.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `AllOrNothing` | Until a merge is final, each file it replaced is old in the folder or kept in its backup. | `keepUntilUndone`: a recovery that could not put every file back removed the backups anyway. `undoTwice`: a recovery run again removed a file the first had put back, taking it for one the merge made | `TestARecoveryThatFailsPartWayIsFinishedLater` |

## Changing a modelled machine

1. Change the spec's action to match the new Go code.
2. Run `spec/check.sh`.
3. When TLC finds a violation, write a Go test that replays the trace with explicit steps, see it fail, then fix the code.
