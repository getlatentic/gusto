--------------------------- MODULE TaskLifecycle ---------------------------
(***************************************************************************)
(* One task in gusto's task index (internal/jobs), as four parties see it: *)
(* the queue's worker (Queue.one), a person stopping it (Queue.Cancel),    *)
(* the run that does its work, and the server crashing and starting again *)
(* (Open).  Each action names the Go function it stands for.               *)
(*                                                                         *)
(* Atomicity assumed:                                                      *)
(*  - an action of the index is one hold of Index.guard;                   *)
(*  - state.json is written whole: torn writes are getlatentic/planning#769;*)
(*  - the run is a child of the container runtime, not of the server, so  *)
(*    it outlives a crash and may still write its manifest.                *)
(*                                                                         *)
(* Protocol "before" is the code before getlatentic/gusto#559, and must    *)
(* fail; "now" is the code since, and must pass.                           *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Protocol,   \* "before" or "now"
    MaxCrashes  \* how many times the server may crash in one behaviour

ASSUME Protocol \in {"before", "now"}
ASSUME MaxCrashes \in Nat

Done == {"Passed", "Failed", "Interrupted"}
States == {"Queued", "Running"} \cup Done

(* The run's manifest (internal/record).  check is its verdict, or "none"   *)
(* for a turn in a conversation; turnOK is that the harness exited 0 and   *)
(* the model answered; stopped is that the run's context was cancelled.    *)
Manifests == [written: BOOLEAN, check: {"pass", "fail", "none"},
              turnOK: BOOLEAN, stopped: BOOLEAN]
NoManifest == [written |-> FALSE, check |-> "none", turnOK |-> TRUE, stopped |-> FALSE]

(* record.Earned: the exit code a run earned. *)
Earned(m) == IF m.check = "pass" THEN 0
             ELSE IF m.check = "fail" THEN 1
             ELSE IF m.turnOK THEN 0 ELSE 1

VARIABLES
    state,          \* Record.State, in memory and in state.json
    stoppedBy,      \* Record.StoppedBy is set
    manifest,       \* out/manifest.json
    up,             \* the server is running
    crashes,        \* crashes so far
    worker,         \* where Queue.one is
    registered,     \* the run's cancel func is in Queue.running
    workExit,       \* what q.work returned: the exit code
    workErr,        \* what q.work returned: an error, no manifest
    sawStop,        \* what Queue.one saw of StoppedBy after the work returned
    run,            \* the run doing the work
    cancelled,      \* the run's context is cancelled
    stopper,        \* where Queue.Cancel is
    stopWasRunning, \* what Index.Stop said
    acked           \* a person was told their stop took

vars == <<state, stoppedBy, manifest, up, crashes, worker, registered, workExit,
          workErr, sawStop, run, cancelled, stopper, stopWasRunning, acked>>

TypeOK ==
    /\ state \in States
    /\ stoppedBy \in BOOLEAN
    /\ manifest \in Manifests
    /\ up \in BOOLEAN
    /\ crashes \in 0..MaxCrashes
    /\ worker \in {"idle", "claimed", "registered", "working", "returned", "looked"}
    /\ registered \in BOOLEAN
    /\ workExit \in {0, 1}
    /\ workErr \in BOOLEAN
    /\ sawStop \in BOOLEAN
    /\ run \in {"none", "running", "ended"}
    /\ cancelled \in BOOLEAN
    /\ stopper \in {"idle", "marked", "done"}
    /\ stopWasRunning \in BOOLEAN
    /\ acked \in BOOLEAN

Init ==
    /\ state = "Queued"
    /\ stoppedBy = FALSE
    /\ manifest = NoManifest
    /\ up = TRUE
    /\ crashes = 0
    /\ worker = "idle"
    /\ registered = FALSE
    /\ workExit = 0
    /\ workErr = FALSE
    /\ sawStop = FALSE
    /\ run = "none"
    /\ cancelled = FALSE
    /\ stopper = "idle"
    /\ stopWasRunning = FALSE
    /\ acked = FALSE

-----------------------------------------------------------------------------
(* The worker: Queue.one. *)

(* Index.Claim: the oldest queued task runs, and an old manifest goes. *)
Claim ==
    /\ up /\ worker = "idle" /\ state = "Queued"
    /\ state' = "Running"
    /\ manifest' = NoManifest
    /\ worker' = "claimed"
    /\ UNCHANGED <<stoppedBy, up, crashes, registered, workExit, workErr, sawStop,
                   run, cancelled, stopper, stopWasRunning, acked>>

(* q.running[id] = cancel *)
Register ==
    /\ up /\ worker = "claimed"
    /\ registered' = TRUE
    /\ worker' = "registered"
    /\ UNCHANGED <<state, stoppedBy, manifest, up, crashes, workExit, workErr, sawStop,
                   run, cancelled, stopper, stopWasRunning, acked>>

(* A stop that came before the cancel func was registered is honoured, then *)
(* q.work starts the run.                                                  *)
StartWork ==
    /\ up /\ worker = "registered"
    /\ cancelled' = (cancelled \/ stoppedBy)
    /\ run' = "running"
    /\ worker' = "working"
    /\ UNCHANGED <<state, stoppedBy, manifest, up, crashes, registered, workExit,
                   workErr, sawStop, stopper, stopWasRunning, acked>>

(* q.work returns, and the cancel func is dropped from Queue.running. *)
WorkReturns ==
    /\ up /\ worker = "working" /\ run = "ended"
    /\ workErr' = ~manifest.written
    /\ workExit' = IF manifest.written THEN Earned(manifest) ELSE 1
    /\ registered' = FALSE
    /\ worker' = "returned"
    /\ UNCHANGED <<state, stoppedBy, manifest, up, crashes, sawStop, run, cancelled,
                   stopper, stopWasRunning, acked>>

(* Index.Get: whether somebody stopped it. *)
LookForStop ==
    /\ up /\ worker = "returned"
    /\ sawStop' = stoppedBy
    /\ worker' = "looked"
    /\ UNCHANGED <<state, stoppedBy, manifest, up, crashes, registered, workExit,
                   workErr, run, cancelled, stopper, stopWasRunning, acked>>

(* The state Queue.one asks Index.Finish for. *)
Asked == IF sawStop THEN "Interrupted"
         ELSE IF workErr \/ workExit # 0 THEN "Failed"
         ELSE "Passed"

(* Index.finish.  Since #559 a task ends once, and a stop recorded after   *)
(* the worker looked still ends it interrupted.                            *)
Finish ==
    /\ up /\ worker = "looked"
    /\ state' = CASE Protocol = "before" -> Asked
                  [] state \in Done      -> state
                  [] stoppedBy           -> "Interrupted"
                  [] OTHER               -> Asked
    /\ worker' = "idle"
    /\ UNCHANGED <<stoppedBy, manifest, up, crashes, registered, workExit, workErr,
                   sawStop, run, cancelled, stopper, stopWasRunning, acked>>

-----------------------------------------------------------------------------
(* A person stopping it: Queue.Cancel. *)

(* Index.Stop: a finished task is refused; a queued one ends interrupted;  *)
(* a running one is marked.  Either way the person is told it took.        *)
Stop ==
    /\ up /\ stopper = "idle"
    /\ IF state \in Done
       THEN /\ stopper' = "done"
            /\ UNCHANGED <<state, stoppedBy, stopWasRunning, acked>>
       ELSE /\ stoppedBy' = TRUE
            /\ stopWasRunning' = (state = "Running")
            /\ state' = IF state = "Queued" THEN "Interrupted" ELSE state
            /\ acked' = TRUE
            /\ stopper' = "marked"
    /\ UNCHANGED <<manifest, up, crashes, worker, registered, workExit, workErr, sawStop,
                   run, cancelled>>

(* The rest of Queue.Cancel: a running task's run is told to stop, if its *)
(* cancel func is still registered.                                        *)
Signal ==
    /\ up /\ stopper = "marked"
    /\ cancelled' = (cancelled \/ (stopWasRunning /\ registered))
    /\ stopper' = "done"
    /\ UNCHANGED <<state, stoppedBy, manifest, up, crashes, worker, registered, workExit,
                   workErr, sawStop, run, stopWasRunning, acked>>

-----------------------------------------------------------------------------
(* The run.  It ends with a manifest, or dies without one.  The manifest   *)
(* says it was stopped when its context was cancelled (runner.stoppedBy).  *)

RunEnds ==
    /\ run = "running"
    /\ run' = "ended"
    /\ \E check \in {"pass", "fail", "none"}, ok \in BOOLEAN :
         manifest' = [written |-> TRUE, check |-> check, turnOK |-> ok, stopped |-> cancelled]
    /\ UNCHANGED <<state, stoppedBy, up, crashes, worker, registered, workExit, workErr,
                   sawStop, cancelled, stopper, stopWasRunning, acked>>

RunDies ==
    /\ run = "running"
    /\ run' = "ended"
    /\ UNCHANGED <<state, stoppedBy, manifest, up, crashes, worker, registered, workExit,
                   workErr, sawStop, cancelled, stopper, stopWasRunning, acked>>

-----------------------------------------------------------------------------
(* The server.  A crash loses the worker and a stop half done; the run and *)
(* everything on disk stay.                                                *)

Crash ==
    /\ up /\ crashes < MaxCrashes
    /\ up' = FALSE
    /\ crashes' = crashes + 1
    /\ worker' = "idle"
    /\ registered' = FALSE
    /\ stopper' = IF stopper = "marked" THEN "done" ELSE stopper
    /\ UNCHANGED <<state, stoppedBy, manifest, workExit, workErr, sawStop, run, cancelled,
                   stopWasRunning, acked>>

(* ended: how a run nobody watched turned out. *)
Recovered ==
    CASE ~manifest.written                          -> "Interrupted"
      [] Protocol = "before" /\ manifest.check = "none" -> "Passed"
      [] Protocol = "before"                        -> IF manifest.check = "pass" THEN "Passed" ELSE "Failed"
      [] manifest.stopped \/ stoppedBy               -> "Interrupted"
      [] OTHER                                      -> IF Earned(manifest) = 0 THEN "Passed" ELSE "Failed"

(* Open: a task the state file says is running is ended from its manifest. *)
Restart ==
    /\ ~up
    /\ up' = TRUE
    /\ state' = IF state = "Running" THEN Recovered ELSE state
    /\ UNCHANGED <<stoppedBy, manifest, crashes, worker, registered, workExit, workErr,
                   sawStop, run, cancelled, stopper, stopWasRunning, acked>>

(* Nothing is left to happen. *)
Finished ==
    /\ state \in Done /\ up /\ worker = "idle" /\ run # "running" /\ stopper # "marked"
    /\ UNCHANGED vars

Next ==
    \/ Claim \/ Register \/ StartWork \/ WorkReturns \/ LookForStop \/ Finish
    \/ Stop \/ Signal
    \/ RunEnds \/ RunDies
    \/ Crash \/ Restart
    \/ Finished

Fairness ==
    /\ WF_vars(Claim) /\ WF_vars(Register) /\ WF_vars(StartWork)
    /\ WF_vars(WorkReturns) /\ WF_vars(LookForStop) /\ WF_vars(Finish)
    /\ WF_vars(Signal) /\ WF_vars(RunEnds \/ RunDies) /\ WF_vars(Restart)

Spec == Init /\ [][Next]_vars /\ Fairness

-----------------------------------------------------------------------------
(* What must hold. *)

(* A person told their stop took never sees the task end any other way. *)
StopWins == acked /\ state \in Done => state = "Interrupted"

(* A task passes only on a manifest that earned it. *)
PassedIsEarned == state = "Passed" => manifest.written /\ Earned(manifest) = 0

(* Graded live or after a crash, a run is graded by the same rule. *)
GradedAsEarned ==
    state \in {"Passed", "Failed"} /\ manifest.written =>
        ((state = "Passed") <=> (Earned(manifest) = 0))

(* A task ends once. *)
EndsOnce == [][(state \in Done) => (state' = state)]_state

(* Every task ends. *)
EveryTaskEnds == <>(state \in Done)
=============================================================================
