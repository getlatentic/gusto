------------------------------- MODULE Retention -----------------------------
(***************************************************************************)
(* One finished task and the sweep that removes it when it is due          *)
(* (internal/retention sweep.go, holds.go; internal/web retention.go).  An *)
(* admin holds the task.  A sweep reads whether the task is held and       *)
(* removes it when it is not.  The task may be a turn of a room, which the *)
(* room's owner removes, and an operator may delete it.  The policy keeps  *)
(* the workspace for ever or for some days, and the record for some days. *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "guard": the sweep reads the holds and removes the task as one step, *)
(*    so a hold placed meanwhile keeps the task;                           *)
(*  - "forever": a policy that removes a record whose workspace is kept    *)
(*    for ever is refused;                                                 *)
(*  - "rooms": a turn of a room that was removed is swept;                 *)
(*  - "forget": a hold on a task that is gone is ended.                    *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes

ASSUME Fixes \subseteq {"guard", "forever", "rooms", "forget"}

VARIABLES
    task,      \* the task is kept
    held,      \* a hold on the task is in force
    swept,     \* the sweep removed a task that was held
    sweep,     \* where a sweep is: idle, or read that the task is not held
    room,      \* the task is a turn of a room: none, kept or removed
    workspace, \* the policy keeps the workspace "forever" or for "days"
    lostWork   \* a workspace meant to be kept for ever was removed

vars == <<task, held, swept, sweep, room, workspace, lostWork>>

TypeOK ==
    /\ task \in BOOLEAN /\ held \in BOOLEAN /\ swept \in BOOLEAN
    /\ sweep \in {"idle", "free"} /\ room \in {"none", "kept", "removed"}
    /\ workspace \in {"forever", "days"} /\ lostWork \in BOOLEAN

(* With forever, a policy that removes a record whose workspace is kept    *)
(* for ever is refused, so the server never runs one.                      *)
Init ==
    /\ task = TRUE /\ held = FALSE /\ swept = FALSE /\ sweep = "idle"
    /\ room \in {"none", "kept"} /\ lostWork = FALSE
    /\ workspace \in IF "forever" \in Fixes THEN {"days"} ELSE {"forever", "days"}

Sweepable == room = "none" \/ (room = "removed" /\ "rooms" \in Fixes)

(* holdJob: the task is there when the admin asks. *)
Hold ==
    /\ task /\ ~held
    /\ held' = TRUE
    /\ UNCHANGED <<task, swept, sweep, room, workspace, lostWork>>

Release ==
    /\ held
    /\ held' = FALSE
    /\ UNCHANGED <<task, swept, sweep, room, workspace, lostWork>>

RemoveRoom ==
    /\ room = "kept"
    /\ room' = "removed"
    /\ UNCHANGED <<task, held, swept, sweep, workspace, lostWork>>

(* Something besides the sweep removes the task, held or not: an operator *)
(* deleting its folder.                                                    *)
Elsewhere ==
    /\ task
    /\ task' = FALSE
    /\ UNCHANGED <<held, swept, sweep, room, workspace, lostWork>>

(* The sweep reads the holds, then removes; with guard both are one step.  *)
Read ==
    /\ sweep = "idle" /\ task /\ Sweepable /\ ~held
    /\ IF "guard" \in Fixes
       THEN /\ task' = FALSE /\ lostWork' = (lostWork \/ workspace = "forever") /\ UNCHANGED sweep
       ELSE /\ sweep' = "free" /\ UNCHANGED <<task, lostWork>>
    /\ UNCHANGED <<held, swept, room, workspace>>

Remove ==
    /\ sweep = "free"
    /\ sweep' = "idle"
    /\ task' = FALSE /\ lostWork' = (lostWork \/ (task /\ workspace = "forever"))
    /\ swept' = (swept \/ (task /\ held))
    /\ UNCHANGED <<held, room, workspace>>

(* After a sweep, a hold on a task that is gone ends. *)
Forget ==
    /\ "forget" \in Fixes /\ ~task /\ held /\ sweep = "idle"
    /\ held' = FALSE
    /\ UNCHANGED <<task, swept, sweep, room, workspace, lostWork>>

Next == Hold \/ Release \/ RemoveRoom \/ Elsewhere \/ Read \/ Remove \/ Forget

Spec == Init /\ [][Next]_vars

FairSpec == Spec /\ WF_vars(Read) /\ WF_vars(Remove) /\ WF_vars(Forget)

-----------------------------------------------------------------------------
(* The sweep never removes a task that is held. *)
TheHeldAreKept == ~swept

(* A workspace the policy keeps for ever is never removed. *)
ForeverIsForever == ~lostWork

(* A due turn of a room that was removed goes, or is held. *)
RemovedRoomsTurnsGo == (room = "removed" /\ task) ~> (~task \/ held)

(* A hold on a task that is gone ends. *)
NoHoldOutlivesItsTask == (~task /\ held) ~> ~held
=============================================================================
