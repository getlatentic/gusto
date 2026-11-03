------------------------------- MODULE Demo --------------------------------
(***************************************************************************)
(* One demo (internal/cloud demo_*.go): made at a visitor's request, its  *)
(* server made ready by the leader's controller, its clock started, used  *)
(* by the visitor or a colleague who joined through the organization's    *)
(* own address, extended once, stopped by the visitor, ended by the       *)
(* leader's sweep when it is due, and the front door crashing at any      *)
(* point.  The lead row is kept after the demo, and says how it went.     *)
(* Each action names the Go function it stands for.                       *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "clockOnReady": the controller starts the clock when the demo's     *)
(*    server is first ready, not a page view of /demo/wait;               *)
(*  - "leadInTx": the lead row changes in the demo's own transaction;     *)
(*  - "guards": a stopped or ending demo is not extended or started, and  *)
(*    a lead's end is written once;                                        *)
(*  - "endingMark": ending marks the demo first, in the database, and a   *)
(*    marked demo is not provisioned again.                                *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Fixes,
    MaxCrashes,
    MaxFailures   \* how many lead writes outside a transaction fail

ASSUME Fixes \subseteq {"clockOnReady", "leadInTx", "guards", "endingMark"}

VARIABLES
    org,          \* the demo's organization: none, live, or gone
    dir,          \* its directory is on disk
    server,       \* its server is running
    ready,        \* demo_ready is set: the clock runs
    extended, stopped, ending,
    phase,        \* the clock: fresh, patienceOver (never ready that long), expired
    leadReady, leadExtended, leadEnded, leadReason,
    sweep,        \* where endDemo is
    wasReady,     \* the server was ready at some point, so the demo was usable
    endBegun,     \* the sweep has begun to end the demo, whatever the code records
    revived,      \* a server or directory was made again for a demo being ended
    extendedStopped, \* a stopped demo was extended
    up, crashes, failures

vars == <<org, dir, server, ready, extended, stopped, ending, phase, leadReady, leadExtended,
          leadEnded, leadReason, sweep, wasReady, endBegun, revived, extendedStopped, up, crashes, failures>>

TypeOK ==
    /\ org \in {"none", "live", "gone"}
    /\ dir \in BOOLEAN /\ server \in BOOLEAN /\ ready \in BOOLEAN
    /\ extended \in BOOLEAN /\ stopped \in BOOLEAN /\ ending \in BOOLEAN
    /\ phase \in {"fresh", "patienceOver", "expired"}
    /\ leadReady \in BOOLEAN /\ leadExtended \in BOOLEAN /\ leadEnded \in BOOLEAN
    /\ leadReason \in {"", "stopped", "expired", "neverReady"}
    /\ sweep \in {"idle", "removed", "dirGone"}
    /\ wasReady \in BOOLEAN /\ endBegun \in BOOLEAN /\ revived \in BOOLEAN /\ extendedStopped \in BOOLEAN
    /\ up \in BOOLEAN /\ crashes \in 0..MaxCrashes /\ failures \in 0..MaxFailures

Init ==
    /\ org = "none" /\ dir = FALSE /\ server = FALSE /\ ready = FALSE
    /\ extended = FALSE /\ stopped = FALSE /\ ending = FALSE /\ phase = "fresh"
    /\ leadReady = FALSE /\ leadExtended = FALSE /\ leadEnded = FALSE /\ leadReason = ""
    /\ sweep = "idle" /\ wasReady = FALSE /\ endBegun = FALSE /\ revived = FALSE /\ extendedStopped = FALSE
    /\ up = TRUE /\ crashes = 0 /\ failures = 0

Guards == "guards" \in Fixes
InTx == "leadInTx" \in Fixes

-----------------------------------------------------------------------------
(* StartDemo: the organization, its directory and its lead. *)
Start ==
    /\ up /\ org = "none"
    /\ org' = "live" /\ dir' = TRUE
    /\ UNCHANGED <<server, ready, extended, stopped, ending, phase, leadReady, leadExtended, leadEnded,
                   leadReason, sweep, wasReady, endBegun, revived, extendedStopped, up, crashes, failures>>

(* DemoReady, in the demo's transaction; the lead row after it, or in it. *)
StartClock ==
    /\ ~ready
    /\ ready' = TRUE
    /\ \/ leadReady' = TRUE /\ UNCHANGED failures
       \/ ~InTx /\ failures < MaxFailures /\ failures' = failures + 1 /\ UNCHANGED leadReady

(* provision: the leader's controller has the server answer.  With        *)
(* clockOnReady it starts the clock then.  The controller refuses a demo *)
(* its Remove forgot (getlatentic/gusto#567), which a crash forgets; with  *)
(* endingMark it refuses one the database marks ending, which a crash     *)
(* does not.                                                               *)
Provision ==
    /\ up /\ org = "live" /\ ~server
    /\ sweep = "idle"
    /\ ~("endingMark" \in Fixes /\ ending)
    /\ server' = TRUE /\ dir' = TRUE /\ wasReady' = TRUE
    /\ revived' = (revived \/ endBegun)
    /\ IF "clockOnReady" \in Fixes /\ ~ready /\ ~(Guards /\ (stopped \/ ending))
       THEN StartClock
       ELSE UNCHANGED <<ready, leadReady, failures>>
    /\ UNCHANGED <<org, extended, stopped, ending, phase, leadExtended, leadEnded, leadReason, sweep,
                   endBegun, extendedStopped, up, crashes>>

(* demoWait: the visitor's page finds the server ready and starts the     *)
(* clock.  A colleague who came by the organization's address does not.   *)
WaitPage ==
    /\ up /\ org = "live" /\ server /\ ~ready
    /\ ~(Guards /\ (stopped \/ ending))
    /\ StartClock
    /\ UNCHANGED <<org, dir, server, extended, stopped, ending, phase, leadExtended, leadEnded,
                   leadReason, sweep, wasReady, endBegun, revived, extendedStopped, up, crashes>>

(* Time passes: patience runs out for a demo whose clock never started,   *)
(* or a started one runs out of time.                                     *)
Tick ==
    /\ org = "live" /\ phase = "fresh"
    /\ phase' = IF ready THEN "expired" ELSE "patienceOver"
    /\ UNCHANGED <<org, dir, server, ready, extended, stopped, ending, leadReady, leadExtended,
                   leadEnded, leadReason, sweep, wasReady, endBegun, revived, extendedStopped, up, crashes, failures>>

(* ExtendDemo: once, while the time runs. *)
Extend ==
    /\ up /\ org = "live" /\ ready /\ ~extended /\ phase = "fresh"
    /\ ~(Guards /\ (stopped \/ ending))
    /\ extended' = TRUE
    /\ extendedStopped' = (extendedStopped \/ stopped)
    /\ \/ leadExtended' = TRUE /\ UNCHANGED failures
       \/ ~InTx /\ failures < MaxFailures /\ failures' = failures + 1 /\ UNCHANGED leadExtended
    /\ UNCHANGED <<org, dir, server, ready, stopped, ending, phase, leadReady, leadEnded, leadReason,
                   sweep, wasReady, endBegun, revived, up, crashes>>

Stop ==
    /\ up /\ org = "live" /\ ~stopped
    /\ stopped' = TRUE
    /\ UNCHANGED <<org, dir, server, ready, extended, ending, phase, leadReady, leadExtended, leadEnded,
                   leadReason, sweep, wasReady, endBegun, revived, extendedStopped, up, crashes, failures>>

-----------------------------------------------------------------------------
(* The reason DemosDue gives, or "" when the demo is not due. *)
Due == CASE stopped -> "stopped"
         [] phase = "expired" -> "expired"
         [] phase = "patienceOver" /\ ~ready -> "neverReady"
         [] OTHER -> ""

(* endDemo: Controller.Remove stops the server (with endingMark, the demo  *)
(* is marked ending first, in its transaction), then Store.EndDemo        *)
(* removes the directory, then deletes the rows and ends the lead.        *)
Remove ==
    /\ up /\ org = "live" /\ sweep = "idle" /\ (Due # "" \/ ending)
    /\ ending' = ("endingMark" \in Fixes \/ ending)
    /\ server' = FALSE
    /\ sweep' = "removed"
    /\ endBegun' = TRUE
    /\ leadReason' = IF Guards /\ leadReason # "" THEN leadReason ELSE IF Due # "" THEN Due ELSE leadReason
    /\ UNCHANGED <<org, dir, ready, extended, stopped, phase, leadReady, leadExtended, leadEnded,
                   wasReady, revived, extendedStopped, up, crashes, failures>>

RemoveDir ==
    /\ up /\ sweep = "removed"
    /\ dir' = FALSE
    /\ sweep' = "dirGone"
    /\ UNCHANGED <<org, server, ready, extended, stopped, ending, phase, leadReady, leadExtended,
                   leadEnded, leadReason, wasReady, endBegun, revived, extendedStopped, up, crashes, failures>>

DeleteRows ==
    /\ up /\ sweep = "dirGone"
    /\ org' = "gone"
    /\ leadEnded' = TRUE
    /\ sweep' = "idle"
    /\ UNCHANGED <<dir, server, ready, extended, stopped, ending, phase, leadReady, leadExtended,
                   leadReason, wasReady, endBegun, revived, extendedStopped, up, crashes, failures>>

(* The front door crashes: the sweep's progress is lost; the database, the *)
(* directory and a server on its host stay.                                *)
Crash ==
    /\ up /\ crashes < MaxCrashes
    /\ up' = FALSE /\ crashes' = crashes + 1 /\ sweep' = "idle"
    /\ UNCHANGED <<org, dir, server, ready, extended, stopped, ending, phase, leadReady, leadExtended,
                   leadEnded, leadReason, wasReady, endBegun, revived, extendedStopped, failures>>

Restart ==
    /\ ~up /\ up' = TRUE
    /\ UNCHANGED <<org, dir, server, ready, extended, stopped, ending, phase, leadReady, leadExtended,
                   leadEnded, leadReason, sweep, wasReady, endBegun, revived, extendedStopped, crashes, failures>>

Next ==
    \/ Start \/ Provision \/ WaitPage \/ Tick \/ Extend \/ Stop
    \/ Remove \/ RemoveDir \/ DeleteRows \/ Crash \/ Restart

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* A demo whose server was ready is never ended as one that never was. *)
UsedIsNotNeverReady == leadReason = "neverReady" => ~wasReady

(* The lead says what the demo did. *)
LeadFollowsDemo == (ready => leadReady) /\ (extended => leadExtended)

(* A stopped demo is not extended. *)
NoExtendAfterStop == ~extendedStopped

(* A demo being ended is not made again. *)
EndingStaysEnding == ~revived
=============================================================================
