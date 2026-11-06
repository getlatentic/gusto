# Specs

TLA+ specifications of gusto's state machines that races and crashes can break. TLC checks each one on a small model.

```bash
spec/check.sh    # fetches tla2tools.jar v1.7.4 once; needs Java 11 or later
```

Expected output:

```
ok   AgentApps.before-forget.cfg: Error: Invariant ChoiceIsOfThisConnection is violated.
ok   AgentApps.before-member.cfg: Error: Invariant ChoiceIsAMembers is violated.
ok   AgentApps.now.cfg: 80 states generated, 39 distinct states found, 0 states left on queue.
ok   AppPreview.before-error.cfg: Error: Invariant NothingRunsUnseen is violated.
ok   AppPreview.before-limit.cfg: Error: Invariant WithinLimit is violated.
ok   AppPreview.before-orphan.cfg: Error: Invariant NothingRunsUnseen is violated.
ok   AppPreview.before-reconcile.cfg: Error: Invariant NoStuckStart is violated.
ok   AppPreview.now.cfg: 1337 states generated, 404 distinct states found, 0 states left on queue.
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
ok   Credentials.before-code.cfg: Error: Invariant ApprovalKept is violated.
ok   Credentials.before-recheck.cfg: Error: Invariant NoStaleAuthority is violated.
ok   Credentials.now.cfg: 308 states generated, 92 distinct states found, 0 states left on queue.
ok   Demo.before-clock.cfg: Error: Invariant UsedIsNotNeverReady is violated.
ok   Demo.before-ending.cfg: Error: Invariant EndingStaysEnding is violated.
ok   Demo.before-guards.cfg: Error: Invariant NoExtendAfterStop is violated.
ok   Demo.before-lead.cfg: Error: Invariant LeadFollowsDemo is violated.
ok   Demo.now.cfg: 388 states generated, 231 distinct states found, 0 states left on queue.
ok   Evidence.before-intact.cfg: Error: Invariant NothingChangedIsSigned is violated.
ok   Evidence.before-pending.cfg: Error: Invariant StaysSigned is violated.
ok   Evidence.now.cfg: 50 states generated, 38 distinct states found, 0 states left on queue.
ok   Formats.before-applied.cfg: Error: Invariant UpToDateIsWhole is violated.
ok   Formats.before-format.cfg: Error: Invariant UndoneIsUpgraded is violated.
ok   Formats.before-scratch.cfg: Error: Invariant StampsLand is violated.
ok   Formats.now.cfg: 325 states generated, 60 distinct states found, 0 states left on queue.
ok   FrontDoors.before-leader.cfg: Error: Invariant OneServer is violated.
ok   FrontDoors.now.cfg: 55269 states generated, 11534 distinct states found, 0 states left on queue.
ok   Helper.before-hold.cfg: Error: Invariant NothingLost is violated.
ok   Helper.before-lock.cfg: Error: Invariant OneHelper is violated.
ok   Helper.now.cfg: 383 states generated, 117 distinct states found, 0 states left on queue.
ok   Hosts.before-busy.cfg: Error: Invariant NoStopUnderAStart is violated.
ok   Hosts.before-mark.cfg: Error: Invariant NoStopUnderAStart is violated.
ok   Hosts.before-unregister.cfg: Error: Invariant DownIsGone is violated.
ok   Hosts.now.cfg: 16509 states generated, 4580 distinct states found, 0 states left on queue.
ok   Install.before-clear.cfg: Error: Invariant RecordIsTrue is violated.
ok   Install.before-stop.cfg: Error: Invariant NoIdleGateway is violated.
ok   Install.before-verify.cfg: Error: Invariant RecordIsTrue is violated.
ok   Install.now.cfg: 70 states generated, 38 distinct states found, 0 states left on queue.
ok   Matrix.before-remake.cfg: Error: Invariant NeverStuck is violated.
ok   Matrix.before-time.cfg: Error: Invariant TimeIsTheRun is violated.
ok   Matrix.now.cfg: 153 states generated, 91 distinct states found, 0 states left on queue.
ok   MergeJournal.before-keep.cfg: Error: Invariant AllOrNothing is violated.
ok   MergeJournal.before-twice.cfg: Error: Invariant AllOrNothing is violated.
ok   MergeJournal.now.cfg: 76 states generated, 50 distinct states found, 0 states left on queue.
ok   OrgSupervisor.before-560.cfg: Error: Invariant EndingStopsServer is violated.
ok   OrgSupervisor.before-provision.cfg: Error: Invariant EndingStopsServer is violated.
ok   OrgSupervisor.now.cfg: 1280 states generated, 411 distinct states found, 0 states left on queue.
ok   Quota.before-mark.cfg: Error: Invariant QuotaFollows is violated.
ok   Quota.before-serial.cfg: Error: Invariant QuotaFollows is violated.
ok   Quota.now.cfg: 5452 states generated, 2777 distinct states found, 0 states left on queue.
ok   Registry.before-lock.cfg: Error: Invariant KeyIsTheLatest is violated.
ok   Registry.before-models.cfg: Error: Invariant RetiredIsRetired is violated.
ok   Registry.now.cfg: 65 states generated, 43 distinct states found, 0 states left on queue.
ok   Retention.before-forever.cfg: Error: Invariant ForeverIsForever is violated.
ok   Retention.before-forget.cfg: Error: Temporal properties were violated.
ok   Retention.before-guard.cfg: Error: Invariant TheHeldAreKept is violated.
ok   Retention.before-rooms.cfg: Error: Temporal properties were violated.
ok   Retention.now.cfg: 26 states generated, 12 distinct states found, 0 states left on queue.
ok   RoomGuard.before-hold.cfg: Error: Invariant Exclusive is violated.
ok   RoomGuard.now.cfg: 162 states generated, 97 distinct states found, 0 states left on queue.
ok   RoomMembers.before-join.cfg: Error: Invariant AddsOnlyAdd is violated.
ok   RoomMembers.before-once.cfg: Error: Invariant OneOwnRoom is violated.
ok   RoomMembers.before-stop.cfg: Error: Invariant NoTurnWithoutItsAgent is violated.
ok   RoomMembers.before-together.cfg: Error: Invariant AgentsFollowTheirOwner is violated.
ok   RoomMembers.now.cfg: 197 states generated, 68 distinct states found, 0 states left on queue.
ok   RoomSocket.before-watch.cfg: Error: Invariant OnlyMembersWatch is violated.
ok   RoomSocket.now.cfg: 24 states generated, 16 distinct states found, 0 states left on queue.
ok   Routines.before-owe.cfg: Error: Invariant NewsKept is violated.
ok   Routines.before-surface.cfg: Error: Invariant NoneMeansNone is violated.
ok   Routines.before-undo.cfg: Error: Invariant NoOrphanThread is violated.
ok   Routines.now.cfg: 1482 states generated, 553 distinct states found, 0 states left on queue.
ok   SpendCap.before-governing.cfg: Error: Invariant BudgetHolds is violated.
ok   SpendCap.before-live.cfg: Error: Invariant BudgetHolds is violated.
ok   SpendCap.before-zero.cfg: Error: Invariant BudgetHolds is violated.
ok   SpendCap.now.cfg: 5532 states generated, 1158 distinct states found, 0 states left on queue.
ok   StateFile.before-dir-sync.cfg: Error: Invariant NothingDoneIsLost is violated.
ok   StateFile.before-in-place.cfg: Error: Invariant AlwaysWhole is violated.
ok   StateFile.before-unsynced.cfg: Error: Invariant AlwaysWhole is violated.
ok   StateFile.now.cfg: 771 states generated, 318 distinct states found, 0 states left on queue.
ok   Subscription.before-tie.cfg: Error: Action property NeverOlder is violated.
ok   Subscription.now.cfg: 39090 states generated, 12753 distinct states found, 0 states left on queue.
ok   TaskLifecycle.before-recovery.cfg: Error: Invariant PassedIsEarned is violated.
ok   TaskLifecycle.before-stop-race.cfg: Error: Invariant StopWins is violated.
ok   TaskLifecycle.now.cfg: 1060 states generated, 577 distinct states found, 0 states left on queue.
ok   WayOut.before-flight.cfg: Error: Invariant OneTrade is violated.
ok   WayOut.before-live.cfg: Error: Invariant NoneAfterEnd is violated.
ok   WayOut.before-mint.cfg: Error: Invariant TokenIsCurrent is violated.
ok   WayOut.before-reread.cfg: Error: Temporal properties were violated.
ok   WayOut.before-session.cfg: Error: Invariant NoNeedlessStart is violated.
ok   WayOut.now.cfg: 3249 states generated, 640 distinct states found, 0 states left on queue.
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

Two front doors on one database, each with a controller over one organization, and two nodes. With `leader`, one front
door at a time holds a lease with an epoch, only it acts, and a runtime write is refused unless its epoch is the lease's.
The lease may lapse while its holder is paused, and the holder acts on until it notices.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `OneServer` | The organization's server runs on one node at most. | `leader`: each front door places it on a node of its own and starts it there. With the lease but no epoch on writes, a paused leader does the same | `TestAControllerThatLostTheLeaseStartsNothing`, `TestLeadershipRunsTheWorkOnOneFrontDoorAtATime` |

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

One person's computer (`internal/computer`, `internal/backend/vm`): its record, its sandbox, the tasks that run in
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

## AppPreview

An agent's shared app previews (`internal/apps`, `internal/web/sharing.go`, `internal/backend/vm/apps.go`): publishing one under the
agent's limit, starting its process in the owner's computer, the proxy finding it not answering, removing it, its
process dying, and the server restarting.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `NothingRunsUnseen` | A process runs only for an app the page shows starting, live or being removed, or while its start is under way. | `orphanStopped`: an app removed while it started was forgotten, and its start's process stayed running and exposed. `errorChecks`: one failed request marked a live app stopped and left its process running | `TestAnAppRemovedWhileItStartsIsStoppedWhenItHasStarted`, `TestOneFailedRequestDoesNotStopALiveApp` |
| `NoStuckStart` | An app shows as starting or being removed only while its start is under way. | `reconcile`: an app a restart cut off stayed starting for good, counting against the limit | `TestARestartFinishesTheAppsItCutOff` |
| `WithinLimit` | An agent has no more apps starting or live than its limit. | `limitInStore`: two publishes at once both passed the count | `TestAppsAddedAtOnceKeepToTheLimit` |

## Credentials

What one person may do on their organization's server, and what they carry to show it (`internal/web/auth.go`, `sent.go`,
`device.go`). The front door changes the person's role, or removes them, and writes the roster the server reads. The person
signs in with a signed cookie that names their role, and approves a computer by the device flow, which gives it a token that
acts as them. Making the token may fail.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `NoStaleAuthority` | No request is served as more than the person is now. | `recheck`: the cookie's role, and a computer's token, held until they expired after the front door demoted or removed the person | `TestAPersonDemotedOrRemovedAtTheFrontDoorIsHeldToItAtTheirNextRequest`, `TestAComputerOfAPersonWhoWasRemovedStopsWorking` |
| `ApprovalKept` | A computer whose approval was taken is given its token, or may ask again. | `keepCode`: the code was spent before the token was made, so a failure made the person start over | `TestAComputerWhoseTokenCouldNotBeMadeAsksAgain` |

## Demo

One demo (`internal/cloud/demo_*.go`): made at a visitor's request, its server made ready by the leader's controller, its
clock started, opened by the visitor or by a colleague through the organization's own address, extended, stopped,
ended by the leader's sweep, and the front door crashing. The lead row is kept after the demo, and says how it went.
A lead write outside a transaction may fail.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `UsedIsNotNeverReady` | A demo whose server was ready is never ended as one that never was. | `clockOnReady`: only the visitor's waiting page started the clock, so a demo a colleague opened was swept as never ready | `TestADemosTimeStartsWhenItsServerAnswers` |
| `LeadFollowsDemo` | The lead says the demo was ready, and extended, when it was. | `leadInTx`: the lead was written after the demo's transaction, and not again when that failed | the model only: the write now cannot fail apart from the demo's |
| `NoExtendAfterStop` | A stopped demo is not extended. | `guards` | `TestAStoppedDemoIsNotExtendedOrStarted` |
| `EndingStaysEnding` | A demo being ended is not made again. | `endingMark`: a crash part way through the end lost the controller's memory of it, and the next start made the demo again | `TestADemoBeingEndedIsNotMadeAgain` |

## Quota

One organization's `quota.json` (`internal/cloud/quota_apply.go`), which its server reads for its limits, and the database it
is worked out from. A Stripe event, the seat sync and a confirmed address change the database, and each then calls
`ApplyQuota`, several at once. Writing the file may fail, and the front door may crash; a start writes every quota again.
Stripe sends an event again when its quota was not written. Role changes and removals are not modelled: an organization with
no admin who confirmed their address is on the trial, which has room for one person, or has a subscription, which decides its
quota.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `QuotaFollows` | When no call is left to make and nothing is marked, the file says what the database does. | `serial`: two calls read in one order and wrote in the other, so the older quota was left | the model only: the order depends on goroutine scheduling |
| `QuotaFollows` | | `dueMark`: a quota a confirmed address or the seat sync failed to write stayed wrong until the next start | `TestAQuotaThatCouldNotBeWrittenIsWrittenByTheSweep` |
| `Converges` | However the changes go, the file comes to say what the database does. | either | |

## SpendCap

One person's monthly budget, as the proxy holds their tasks to it (`pkg/spend/ledger.go`, `cmd/gusto/wayout.go`). An admin sets
the budget, lowers it, or sets $0. The person's tasks start and make calls, priced or not, and `Ledger.Allow` decides each one.
After an answer that carried no usage, a task's later calls cannot be counted. The server restarts, and `Rebuild` gives each task
its terms again. Calls already in flight may take a person past the budget, so the property is about what `Allow` lets leave.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `BudgetHolds` | No call leaves that the budget, as the settings hold it now, should stop. | `live`: the budget was read when the task started, so lowering it did not stop running tasks, and a restart dropped it | `TestALoweredBudgetStopsATaskAlreadyRunning` |
| `BudgetHolds` | | `governing`: a person's budget alone let through calls it could not count | `TestABudgetOrAnAllowanceAloneRefusesWhatItCannotCount` |
| `BudgetHolds` | | `zero`: one person's $0 budget was taken for no budget | `TestOnePersonsBudgetOfNothingAllowsNothing` |

## Subscription

One organization's subscription as Stripe changes it and as `RecordSubscription` keeps it. Stripe stamps each change's event
with the second it was made in, and sends it until it is answered, in any order, to handlers that run at once. The model's
stamps are Checkout's: three changes in one second, then one later.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `NeverOlder` | The row never goes back to an older version of the subscription. | `tieFetch`: of two events in one second, the one that arrived last was kept, so an active subscription went back to incomplete | `TestTwoEventsInOneSecondKeepTheSubscriptionAsStripeHasIt` |
| `Converges` | Once Stripe stops changing it, the row comes to hold the subscription as it is. | `tieFetch` | |

## Routines

One agent's routines (`internal/instance/routine.go`, `internal/web/routines.go`). A routine's slot comes, and it starts a run
or skips while its last run is still answering; the slot is kept as the routine's last. A run's turn ends, and its news is
offered to the channels until it is taken or given up on. A person adds a routine, which opens its thread and then writes
`routines.json`, and the write may fail. The file may be damaged from outside: writes are whole or not at all (StateFile).

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `NewsKept` | Every run's news is settled, or still kept to be told. | `owe`: a slot replaced the routine's last run, so a skip beside a run still answering, or the next run, lost its news | `TestARunsNewsIsToldAfterASlotItWasStillAnsweringIn`, `TestARunsNewsIsToldAfterTheNextRunStarts` |
| `NoOrphanThread` | Every routine thread belongs to a routine. | `undo`: an add whose write failed left the thread it opened | `TestARoutineThatCannotBeKeptLeavesNoThread` |
| `NoneMeansNone` | A read that gives no routines is of a file that holds none. | `surface`: a damaged `routines.json` was read as no routines, so every routine stopped and the page showed none | `TestADamagedRoutinesFileIsSaidOnThePage` |

## Hosts

The hosts organizations' servers run on, as the scaler stops them and the controller places on them
(`internal/node/scaler.go`, `internal/node/pool.go`, `internal/cloud/controller.go`). An organization is provisioned: it is
placed on a host, or stays on the one it was placed on, and its server is started there. A server may end on its own. The
scaler retires a host it judges idle: it marks it, checks it again and stops it. A stopped host may start again and register.
While a host's registration is there, the pool takes a silent host to be away for a while; once it is not, the host is gone,
and its organizations are placed again.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `DownIsGone` | An organization on a host that is down is not kept there: the pool says the host is gone. | `unregister`: a host the scaler stopped or removed stayed registered, so its organizations were tried there for half an hour | `TestAHostTheScalerStopsOrRemovesIsGoneToThePool` |
| `NoStopUnderAStart` | No host is stopped under an organization whose server is being started on it. | `busy`: an organization being placed or started did not count, and the idle host is the one with the most room | `TestAHostIsNotStoppedUnderAnOrganizationBeingPlacedOnIt` |
| `NoStopUnderAStart` | | `mark`: a host was placed on after its last check and before it stopped | `TestAHostBeingStoppedIsNotPlacedOnWhileItBeatsOnItsWayDown` |

Two more fixes have Go tests and no model: a front door that starts again counts on from the day's prewarms
(`TestTheDaysPrewarmsAreCountedOnByAFrontDoorThatStartsAgain`), and knows when a quiet host was last heard from
(`TestAHostThatWentQuietAMomentAgoIsLeftByAFrontDoorThatStartsAgain`). A host that goes stops its servers together, inside
its service's stop time (`TestAHostThatGoesStopsItsServersTogetherAndTakesItsRegistrationAway`).

## Evidence

A room turn's record, as `evidence.Decide` puts what was done with the turn's changes under its signature
(`internal/evidence/decide.go`). The run ends with `out/` hashed into `manifest.json` and the manifest signed. Decide writes
`decision.json`, makes the manifest again and signs it again. A file in `out/` may be changed after the run was signed, and
the server may crash at any step and start again.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `StaysSigned` | A signed record that Decide is done with is signed over its manifest. | `pending`: the manifest was written without its signature first, so a crash or a key that could not sign left the record unsigned for good | `TestADecisionTheKeyCannotSignLeavesTheRecordSignedAsItWas`, `TestADecisionCutShortIsSaidAndFinished` |
| `NothingChangedIsSigned` | The signature never vouches for a file changed after the run was signed. | `intact`: Decide hashed `out/` as it was and signed it | `TestARecordChangedSinceItWasSignedIsNotSignedAgain` |

## Matrix

One cell of a benchmark matrix (`internal/matrix/matrix.go`, `internal/matrix/prepare.go`). A measure readies the cell and
runs it when it has no verdict or is asked for again; a used directory is moved aside and made again first. A run may leave
a verdict, and may fail after it. The measure may stop at any step, and is run again.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `NeverStuck` | No run is started in a directory that has no `job.json`. | `remake`: a stop between moving the old run aside and making the cell again failed every later measure | `TestACellMovedAsideAndNotMadeAgainIsMadeAndRun` |
| `TimeIsTheRun` | A cell whose run left a verdict is timed around that run. | `time`: a run that failed after its verdict was not timed, so the cell kept the time of the run it replaced | `TestACellsTimeIsTheTimeOfTheRunItHolds` |

Two more fixes from the same pass have Go tests and no model. A comparison is listed whole from the moment its id is
reserved, and one refused for any agent queues no run (`TestAReservedLineupIsListedWhole`,
`TestAComparisonRefusedForOneAgentQueuesNoRun`). A replay writes its mark first, so one cut short is never swept as an
original (`TestAReplayCutShortIsNotTakenForAnOriginal`).

## Helper

The desktop helper on one computer (`internal/helper/daemon.go`, `internal/helper/mirror/engine.go`). Helpers start: each
looks for one already running and, finding none, says where it is and serves. Engines keep a pair of folders the same, the
helper's and a sync beside it: each run reads the folders' record, works and writes it again.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `OneHelper` | One helper serves at a time. | `lock`: two helpers started together both looked before either said where it was, and both served | `TestAHelperStartedBesideOneThatIsStartingRefuses` |
| `NothingLost` | No run writes the folders' record over a run it did not read. | `hold`: an engine wrote what it read when it was made, so a file the other engine carried and the person deleted came back | `TestTwoEnginesOfOneFolderEachReadWhatTheOtherLeft` |

Two more fixes have Go tests and no model. A token kept in the file wins over an older one in the keychain
(`TestATokenKeptInTheFileWinsOverAnOlderOneInTheKeychain`). An engine stopped for a new folder path or agent stays among the
running until it ends, and is then started again at once (`TestAFolderRegisteredAtANewPathRestartsItsEngineOnceTheOldOneHasEnded`).

## Install

The OpenShell install on a person's computer, made and run again for a new release (`internal/vmwall/apply.go`). An install
puts the programs in place, keeps the agents' root file system, starts the gateway for good, and last writes `vm.json`,
which is what gusto uses. Any step may fail, and the install stops there.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `RecordIsTrue` | What `vm.json` names is what is in place. | `clear`: a re-run that failed left the old `vm.json` beside the new programs | `TestAReRunThatFailsIsNotUsedAndStopsTheGateway` |
| `RecordIsTrue` | | `verify`: a root file system already there was kept, though its name is the processor's and not the release's | `TestANewReleasesImageReplacesTheOneKeptBefore` |
| `NoIdleGateway` | No gateway runs that no install gusto uses started. | `stop`: an install that failed left its gateway, or the one before it, started at every login | `TestAReRunThatFailsIsNotUsedAndStopsTheGateway` |

The runner image fixes from the same pass have Go tests and no model. A server whose start found no image tries the fetch
again until it is here (`TestAFailedImageFetchIsTriedAgainUntilTheImageIsHere`), and a load whose naming failed is named,
not fetched again (`TestALoadedImageThatCouldNotBeNamedIsNamedAndNotFetchedAgain`). A crash between starting the gateway and
writing `vm.json` leaves a gateway that the next install starts again over, and `gusto local sandbox uninstall` stops.

## Registry

One provider in an organization's model registry, and its model, as admins change them at once (`internal/registry/edit.go`).
Adding the provider checks its name is free, keeps the key it was given, then keeps the provider, and forgets the key when
that is refused. Retiring it keeps it retired, then forgets its key.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `KeyIsTheLatest` | Once every change has finished, the key kept is the one the provider was last added with. | `lock`: a refused add forgot the key of the one that won, and a late forget took the key of a provider added again | `TestTwoAdminsAddingTheSameProviderAtOnceLeaveTheOneAddedWithItsKey`, `TestARetiredProviderAddedAgainKeepsItsNewKey` |
| `RetiredIsRetired` | A retired provider has no model in use, and none of its models is changed. | `models`: retiring a provider left its models live, to be priced and turned on | `TestARetiredProvidersModelsAreRetiredWithItAndCannotBeChanged`, `TestAModelKeptLiveUnderARetiredProviderCannotBeChanged` |

## Formats

The format of what gusto keeps (`internal/datadir/datadir.go`, `internal/dbkit/migrate.go`). A data directory has data in a
format and a stamp that says which, and one made before stamps is format 1 with no stamp. A database has had some of a
component's migrations, and a migration can be added later with a number below one it has had.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `UndoneIsUpgraded` | No step is undone on data in another format. | `format`: a rollback took a directory with data and no stamp as the current format, and undid steps it never had | `TestADirectoryWithDataAndNoStampIsFormatOneToARollbackToo` |
| `StampsLand` | Every stamp written is put in place. | `scratch`: two releases stamping at once wrote the same scratch file, and the second rename found none | `TestTwoReleasesStampingADirectoryAtOnceBothStampIt` |
| `UpToDateIsWhole` | A database said to be up to date has had every migration. | `applied`: a migration numbered below the newest applied was passed over | `TestADatabaseThatPassedOverAMigrationIsRefused`, `TestADatabaseThatHadAnotherMigrationUnderANumberIsRefused`, `TestAMigrationSetWithAGapIsRefused` |

## AgentApps

What one person chose for an agent and one app of theirs (`internal/crew/crew.go`, `internal/web/connectors.go`). The person
works with the agent or not, as its owner decides, and is connected to the app or not. A turn they ask of the agent uses the
app by their choice. Each connection is numbered.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `ChoiceIsAMembers` | A choice is held only for a person who works with the agent. | `member`: anybody chose for an agent, and a person taken off it kept their choices for when they were let back | `TestAPersonWhoDoesNotWorkWithAnAgentDoesNotChooseForIt`, `TestAPersonTakenOffAnAgentLosesWhatTheyChoseForItsApps` |
| `ChoiceIsOfThisConnection` | A choice that a turn would use was made in the connection it uses. | `forget`: an app connected again was used by a choice made under the connection that ended | `TestAnAppConnectedAgainFollowsThePersonsDefaultAndNotWhatTheyChoseBefore` |

## RoomMembers

Who and what is in one room (`internal/instance/membership.go`, `internal/web/rooms_members.go`, `internal/web/members_agents.go`).
An admin adds a person, and their agent may be added with them; another admin may make that person an admin at the same time.
The person can be taken out, and their agent goes with them. The agent takes turns and can be deleted. The person opens their
own room with the agent from two clicks at once, and the server can crash between two steps.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `AddsOnlyAdd` | No add takes an admin's role away, and one that is refused adds nobody. | `join`: the add checked a room read before, and added people one at a time | `TestAnAddThatIsRefusedAddsNobody`, `TestJoiningIsAllOrNothingAndNeverChangesARole` |
| `AgentsFollowTheirOwner` | An agent whose owner is not in the room is not in it either. | `together`: a crash after taking the person out left their agents in | `TestAPersonTakenOutOfARoomTakesTheirAgentsInTheSameChange` |
| `NoTurnWithoutItsAgent` | No turn runs for an agent that is not in the room. | `stop`: deleting an agent took it out of its rooms and left its turns running | `TestDeletingAnAgentStopsItsTurnsInTheRoomsItWasIn` |
| `OneOwnRoom` | A person has one room of their own with an agent. | `once`: two clicks each found none and made one | `TestMessagingAnAgentTwiceAtOnceOpensOneRoom`, `TestAPersonsOwnRoomWithAnAgentIsMadeOnceAndWhole` |

The model takes stopping a turn as part of the change that takes its agent out. In the code it follows the change at once, and
a crash between them ends the turn with the process. Accepting an agent that is working already is refused, with a Go test
(`TestAnAgentThatIsWorkingIsNotAcceptedAgain`) and no model.

## Retention

One finished task and the sweep that removes it when it is due (`internal/retention/sweep.go`, `internal/retention/holds.go`,
`internal/web/retention.go`). An admin holds the task. A sweep reads whether the task is held and removes it when it is not.
The task may be a turn of a room, which its owner removes, and an operator may delete it. The policy keeps the workspace for
ever or for some days.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `TheHeldAreKept` | The sweep never removes a task that is held. | `guard`: the sweep read the holds once, and removed a task held after that | `TestAHoldPlacedWhileASweepRunsKeepsTheTask` |
| `ForeverIsForever` | A workspace the policy keeps for ever is never removed. | `forever`: a record with days and a workspace with none took the workspace with it | `TestARecordCannotGoWhileItsWorkspaceIsKeptForEver` |
| `RemovedRoomsTurnsGo` | A due turn of a room that was removed goes, or is held. | `rooms`: no turn of a room was ever swept, and removing a room keeps its turns | `TestATurnOfARoomThatWasRemovedIsSweptAndOneOfAKeptRoomIsNot` |
| `NoHoldOutlivesItsTask` | A hold on a task that is gone ends. | `forget`: holds on removed tasks stayed for ever | `TestAHoldOnATaskThatIsGoneIsEnded` |

## WayOut

A task's ways out of its sandbox: the tool gateway (`pkg/gateway`) and the model proxy's tokens (`pkg/proxy/tokens.go`,
`internal/runtoken`). The gateway reads a server's tools when a request first asks, and two calls share one session with the
server, which the server may lose. The proxy mints a task's token again when its run restarts with another credential, and a
run's forwarder trades its token near its end, from calls made at once.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `UpIsRead` | A server that is up has its tools read. | `reread`: the first request's cancellation, or a server down then, left it broken for the task | `TestAServerIsNotBrokenForTheTaskByAFirstRequestThatWasCancelled`, `TestAServerThatCouldNotBeReadIsReadAgainLater` |
| `NoNeedlessStart` | A session is started again only when the server lost one. | `session`: a call told its session was lost ended the one another call had started | `TestALostSessionEndsOnlyThatSession` |
| `TokenIsCurrent` | A task's token is good for the credential its run was started with. | `mint`: minting again kept the old credential | `TestATaskMintedAgainKeepsItsTokenAndTakesTheNewCredential` |
| `NoneAfterEnd` | No token is minted for a run that has ended. | `live`: a refresh minted for any run whose token was still good | `TestARunThatHasEndedIsNotGivenANewToken` |
| `OneTrade` | One trade of a run's token at a time. | `flight`: every call near the token's end traded it | `TestManyCallsAsATokenNearsItsEndRefreshItOnce` |

`MintBound` marks the token bound in the same step that mints it, so a bound task's secret never stands as a token, which has no
test: there is no moment to catch. A record that cannot be written stops every call after it for the rest of the task. That is
kept: a call that is not written down cannot be audited, and a later line does not make up for the missing one.

## RoomSocket

One person's socket subscribing to a room while they are taken out of it (`internal/roomlive/connect.go`). The subscription
checks the person is in the room, and taking them out asks every socket watching the room to be checked again, which cannot
unsubscribe a socket that is not subscribed yet.

| Property | What it says | Broken without | Go test |
| --- | --- | --- | --- |
| `OnlyMembersWatch` | Once the check owed is done, a subscribed socket is a member's. | `watch`: a socket checked before it was a watcher was subscribed after the check that missed it | `TestAMemberTakenOutWhileTheirSocketSubscribesLosesTheRoom` |

The other findings of the same pass have Go tests and no model, since each is one step:

| Finding | Go test |
| --- | --- |
| Turning off an integration that is off writes nothing down, and its programs are asked for again when it comes back | `TestTurningOffWhatIsOffChangesNothingAndWritesNothingDown`, `TestProgramsAllowedBeforeATurnOffAreAskedForAgainWhenItComesBack` |
| A change to an integration records what it replaced, under the store's lock | `TestEachChangeSaysWhatItReplaced` |
| A page shows a message from its address only when this server sealed it | `TestAMessageInAnAddressIsShownOnlyWhenThisServerSaidIt` |
| What the add dialog refuses is said in the add dialog | `TestWhatTheAddDialogRefusesIsSaidInTheAddDialog` |
| An expired invitation is not listed as waiting | `TestAnExpiredInvitationIsNotListedAsWaiting` |
| A backfill reads a task with a gap again from its first line | `TestABackfillFillsAGapTheRecorderLeftThoughLaterLinesAreHeld` |
| A turn the agent ended for another reason than being done is not an answer | `TestATurnTheAgentEndedWithoutAnsweringIsNotAnAnswer` |

## Changing a modelled machine

1. Change the spec's action to match the new Go code.
2. Run `spec/check.sh`.
3. When TLC finds a violation, write a Go test that replays the trace with explicit steps, see it fail, then fix the code.
