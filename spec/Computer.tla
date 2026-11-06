----------------------------- MODULE Computer ------------------------------
(***************************************************************************)
(* One person's computer in an organization (internal/computer and        *)
(* internal/backend/vm): its record, its sandbox on the gateway, the      *)
(* tasks that run in it, the front door asking for it to be removed when  *)
(* the person is, and the organization server's tend loop, which removes  *)
(* what was asked and stops what is idle.  The record keeps one id for    *)
(* the person, so a computer made again after one was removed is a new    *)
(* generation under the same id.  Each action names the Go function it    *)
(* stands for.                                                             *)
(*                                                                         *)
(* Tasks are paid for by the computer's owner and may be asked by a       *)
(* colleague, as a room agent's turn is.                                   *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "retireOnlyLive": Retire marks only a live computer, and Ensure     *)
(*    clears a mark left on one that was removed;                          *)
(*  - "failBeforeStart": a task that fails before its sandbox starts      *)
(*    marks the computer Failed, so it does not stay Creating;             *)
(*  - "inUse": the computer counts the tasks in it, under its own lock,   *)
(*    and the tend loop stops only one with none, under that lock.         *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets

CONSTANTS
    Fixes,      \* a subset of {"retireOnlyLive", "failBeforeStart", "inUse"}
    Tasks,      \* the tasks
    Askers,     \* who asks a task: the owner, or a colleague
    MaxGen,     \* how many times the computer may be made
    MaxRuns     \* how many task runs there are in all

ASSUME Fixes \subseteq {"retireOnlyLive", "failBeforeStart", "inUse"}

VARIABLES
    gen,        \* the record's generation, or 0 for none
    state,      \* the record's state
    marked,     \* the .retire mark is there
    sandbox,    \* the sandbox on the gateway: none, running, stopped
    pc,         \* where each task is
    asker,      \* who asked each task
    inUse,      \* with "inUse", the tasks counted in the computer
    tender,     \* where the tend loop's idle stop is
    asked,      \* the generations a removal was asked for, while they were live
    killed,     \* a generation was removed that nobody asked to remove
    stoppedMid, \* the sandbox was stopped while a task worked in it
    runs

vars == <<gen, state, marked, sandbox, pc, asker, inUse, tender, asked, killed, stoppedMid, runs>>

States == {"none", "Creating", "Running", "Stopped", "Failed", "Deleted"}
Live == state \notin {"none", "Deleted"}
Working == {t \in Tasks : pc[t] \in {"started"}}

TypeOK ==
    /\ gen \in 0..MaxGen /\ state \in States /\ marked \in BOOLEAN
    /\ sandbox \in {"none", "running", "stopped"}
    /\ pc \in [Tasks -> {"idle", "ensured", "started"}]
    /\ asker \in [Tasks -> Askers]
    /\ inUse \in SUBSET Tasks
    /\ tender \in {"idle", "chose"}
    /\ asked \subseteq 1..MaxGen /\ killed \in BOOLEAN /\ stoppedMid \in BOOLEAN
    /\ runs \in 0..MaxRuns

Init ==
    /\ gen = 0 /\ state = "none" /\ marked = FALSE /\ sandbox = "none"
    /\ pc = [t \in Tasks |-> "idle"] /\ asker \in [Tasks -> Askers]
    /\ inUse = {} /\ tender = "idle"
    /\ asked = {} /\ killed = FALSE /\ stoppedMid = FALSE /\ runs = 0

UsesInUse == "inUse" \in Fixes

-----------------------------------------------------------------------------
(* EnsureFrom: the person's computer, made anew when there is none live. *)
Ensure(t) ==
    /\ pc[t] = "idle" /\ runs < MaxRuns
    /\ IF Live
       THEN UNCHANGED <<gen, state, marked>>
       ELSE /\ gen < MaxGen
            /\ gen' = gen + 1 /\ state' = "Creating"
            /\ marked' = IF "retireOnlyLive" \in Fixes THEN FALSE ELSE marked
    /\ pc' = [pc EXCEPT ![t] = "ensured"]
    /\ runs' = runs + 1
    /\ \E who \in Askers : asker' = [asker EXCEPT ![t] = who]
    /\ UNCHANGED <<sandbox, inUse, tender, asked, killed, stoppedMid>>

(* admitSandbox, NewComputer or recordRun fails before the sandbox starts. *)
FailBeforeStart(t) ==
    /\ pc[t] = "ensured"
    /\ pc' = [pc EXCEPT ![t] = "idle"]
    /\ state' = IF "failBeforeStart" \in Fixes /\ state = "Creating" THEN "Failed" ELSE state
    /\ UNCHANGED <<gen, marked, sandbox, asker, inUse, tender, asked, killed, stoppedMid, runs>>

(* computerBox.Start: the sandbox starts, or starts again when stopped,  *)
(* and the record says Running.  With "inUse" the task is counted first, *)
(* under the computer's lock.                                             *)
Start(t) ==
    /\ pc[t] = "ensured" /\ state # "Deleted"
    /\ UsesInUse => tender = "idle"
    /\ sandbox' = "running"
    /\ state' = "Running"
    /\ inUse' = IF UsesInUse THEN inUse \cup {t} ELSE inUse
    /\ pc' = [pc EXCEPT ![t] = "started"]
    /\ UNCHANGED <<gen, marked, asker, tender, asked, killed, stoppedMid, runs>>

(* computerBox.Close: the task ends; a removed computer stays removed. *)
Close(t) ==
    /\ pc[t] = "started"
    /\ state' = IF state \in {"Deleted", "Failed"} THEN state ELSE "Running"
    /\ inUse' = inUse \ {t}
    /\ pc' = [pc EXCEPT ![t] = "idle"]
    /\ UNCHANGED <<gen, marked, sandbox, asker, tender, asked, killed, stoppedMid, runs>>

-----------------------------------------------------------------------------
(* The front door removes the person: Retire marks their computer.  The *)
(* record is read, but whether it is live is not, unless retireOnlyLive.  *)
Remove ==
    /\ state # "none" /\ ~marked
    /\ IF "retireOnlyLive" \in Fixes /\ ~Live
       THEN UNCHANGED <<marked, asked>>
       ELSE /\ marked' = TRUE
            /\ asked' = IF Live THEN asked \cup {gen} ELSE asked
    /\ UNCHANGED <<gen, state, sandbox, pc, asker, inUse, tender, killed, stoppedMid, runs>>

(* retire, in the tend loop: a marked, live computer's sandbox is deleted *)
(* and its record says Deleted (Retired).                                 *)
Retire ==
    /\ marked /\ Live
    /\ killed' = (killed \/ gen \notin asked)
    /\ sandbox' = "none"
    /\ state' = "Deleted"
    /\ marked' = FALSE
    /\ UNCHANGED <<gen, pc, asker, inUse, tender, asked, stoppedMid, runs>>

(* rest, in the tend loop: a Running computer nobody is busy in is chosen, *)
(* then stopped.  Without "inUse", busy is whether a task the owner asked *)
(* is running, read before the stop.  With it, the choice and the stop are *)
(* under the computer's lock, with no task counted in it.                  *)
Busy == IF UsesInUse THEN inUse # {}
        ELSE \E t \in Tasks : pc[t] = "started" /\ asker[t] = "owner"

Choose ==
    /\ tender = "idle" /\ state = "Running" /\ sandbox = "running" /\ ~Busy
    /\ tender' = "chose"
    /\ UNCHANGED <<gen, state, marked, sandbox, pc, asker, inUse, asked, killed, stoppedMid, runs>>

StopIdle ==
    /\ tender = "chose"
    /\ stoppedMid' = (stoppedMid \/ Working # {})
    /\ sandbox' = IF sandbox = "running" THEN "stopped" ELSE sandbox
    /\ state' = IF state = "Running" THEN "Stopped" ELSE state
    /\ tender' = "idle"
    /\ UNCHANGED <<gen, marked, pc, asker, inUse, asked, killed, runs>>

Next ==
    \/ \E t \in Tasks : Ensure(t) \/ FailBeforeStart(t) \/ Start(t) \/ Close(t)
    \/ Remove \/ Retire \/ Choose \/ StopIdle

Fairness ==
    /\ \A t \in Tasks : WF_vars(Start(t)) /\ WF_vars(Close(t))
    /\ WF_vars(Retire) /\ WF_vars(StopIdle)

Spec == Init /\ [][Next]_vars /\ Fairness

-----------------------------------------------------------------------------
(* A computer is removed only when its person was removed while it lived. *)
OnlyAskedIsRemoved == ~killed

(* No sandbox is stopped while a task works in it. *)
NoStopMidTask == ~stoppedMid

(* A computer is Creating only while a task is making it: one that no   *)
(* task is under way in would count against the quota and show as being *)
(* made for good.                                                         *)
NoIdleCreating == state = "Creating" => \E t \in Tasks : pc[t] = "ensured"
=============================================================================
