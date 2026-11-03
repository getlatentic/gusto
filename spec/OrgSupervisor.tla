--------------------------- MODULE OrgSupervisor ---------------------------
(***************************************************************************)
(* One organization address (slug) in the front door's controller          *)
(* (internal/cloud/controller.go), across the generations of organization *)
(* that may hold it: a demo's slug has two random bytes, so a slug can be *)
(* given again after its demo ends.  Each action names the Go function it *)
(* stands for.                                                             *)
(*                                                                         *)
(* Who provisions: the signup's goroutine (route.go provision), Run's     *)
(* goroutines at start, and the supervisor (restart and reprovision).     *)
(* Who removes: the demo sweep's endDemo, which is Controller.Remove and  *)
(* then Store.EndDemo.                                                     *)
(*                                                                         *)
(* Atomicity assumed:                                                      *)
(*  - provision holds the slug's lock throughout, as Remove does, so the  *)
(*    body of one provision is one step;                                   *)
(*  - Store.EndDemo deletes the directory, then the rows in one           *)
(*    transaction: one step, since nothing that matters reads in between;  *)
(*  - a server on a host apart outlives the front door.                    *)
(*                                                                         *)
(* Fixes is which fixes the code has.  "restartChecksKept" is             *)
(* getlatentic/gusto#560; "provisionChecksOrg" is that provision refuses  *)
(* an organization that is gone or was removed.                            *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Fixes,       \* a subset of {"restartChecksKept", "provisionChecksOrg"}
    MaxGen,      \* how many organizations may hold the slug, one after another
    MaxCrashes   \* how many times the front door may crash

ASSUME Fixes \subseteq {"restartChecksKept", "provisionChecksOrg"}
ASSUME MaxGen \in Nat \ {0} /\ MaxCrashes \in Nat

Gens == 1..MaxGen
MemStates == {"Starting", "Ready", "Failed", "Waiting"}

VARIABLES
    org,         \* the generation the store's orgs row is, or 0 for none
    made,        \* how many generations have been made
    dir,         \* the organization's directory is on disk
    runtimeRow,  \* the store's runtime row for the slug is there
    kept,        \* Controller.states has the slug
    memState,    \* its state there
    removed,     \* the generation Controller.Remove last forgot, or 0
    server,      \* the slug's server runs on its cell
    up,          \* the front door is running
    crashes,     \* front-door crashes so far
    signup,      \* the signup's Provision goroutine has yet to run
    runCall,     \* a Provision goroutine Run started has yet to run
    supervisor,  \* what supervise decided under c.mu and has yet to do
    ender,       \* where endDemo is: idle, removed, or ended in the store
    ending       \* the generation endDemo is ending

vars == <<org, made, dir, runtimeRow, kept, memState, removed, server, up, crashes,
          signup, runCall, supervisor, ender, ending>>

TypeOK ==
    /\ org \in 0..MaxGen /\ made \in 0..MaxGen
    /\ dir \in BOOLEAN /\ runtimeRow \in BOOLEAN
    /\ kept \in BOOLEAN /\ memState \in MemStates
    /\ removed \in 0..MaxGen
    /\ server \in BOOLEAN
    /\ up \in BOOLEAN /\ crashes \in 0..MaxCrashes
    /\ signup \in BOOLEAN /\ runCall \in BOOLEAN
    /\ supervisor \in {"idle", "restart", "wait"}
    /\ ender \in {"idle", "removed", "ended"} /\ ending \in 0..MaxGen

Init ==
    /\ org = 0 /\ made = 0
    /\ dir = FALSE /\ runtimeRow = FALSE
    /\ kept = FALSE /\ memState = "Starting"
    /\ removed = 0
    /\ server = FALSE
    /\ up = TRUE /\ crashes = 0
    /\ signup = FALSE /\ runCall = FALSE
    /\ supervisor = "idle"
    /\ ender = "idle" /\ ending = 0

-----------------------------------------------------------------------------
(* provision: the body under the slug's lock.  With provisionChecksOrg,   *)
(* stillServed refuses an organization that is gone or that Remove forgot, *)
(* and forgets the slug.  Without it, provision goes on for one that is    *)
(* gone: Store.Runtime gives a zero runtime, set writes a row, prepare     *)
(* makes the directory, and the server starts.  The outcome is Ready,      *)
(* Failed, or Waiting for a cell.                                          *)
Refused ==
    "provisionChecksOrg" \in Fixes /\ (org = 0 \/ removed = org)

Provision ==
    IF Refused
    THEN /\ kept' = FALSE
         /\ UNCHANGED <<dir, runtimeRow, memState, server>>
    ELSE /\ kept' = TRUE
         /\ runtimeRow' = TRUE
         /\ dir' = TRUE
         /\ \/ memState' = "Ready" /\ server' = TRUE
            \/ memState' = "Failed" /\ server' = server
            \/ memState' = "Waiting" /\ server' = server

(* reprovision: provision, for a slug the controller still keeps. *)
Reprovision ==
    IF kept THEN Provision ELSE UNCHANGED <<dir, runtimeRow, kept, memState, server>>

-----------------------------------------------------------------------------
(* Signup: a new organization takes the slug, and its server is started   *)
(* without the person waiting (route.go provision).                        *)
Create ==
    /\ up /\ org = 0 /\ made < MaxGen
    /\ org' = made + 1 /\ made' = made + 1
    /\ dir' = TRUE
    /\ signup' = TRUE
    /\ UNCHANGED <<runtimeRow, kept, memState, removed, server, up, crashes, runCall,
                   supervisor, ender, ending>>

SignupProvision ==
    /\ up /\ signup
    /\ Provision
    /\ signup' = FALSE
    /\ UNCHANGED <<org, made, removed, up, crashes, runCall, supervisor, ender, ending>>

RunProvision ==
    /\ up /\ runCall
    /\ Provision
    /\ runCall' = FALSE
    /\ UNCHANGED <<org, made, removed, up, crashes, signup, supervisor, ender, ending>>

-----------------------------------------------------------------------------
(* supervise: under c.mu it picks what to restart, then acts outside it.  *)
Supervise ==
    /\ up /\ supervisor = "idle" /\ kept
    /\ \/ (memState = "Ready" /\ ~server) /\ supervisor' = "restart"
       \/ memState = "Failed" /\ supervisor' = "restart"
       \/ memState = "Waiting" /\ supervisor' = "wait"
    /\ UNCHANGED <<org, made, dir, runtimeRow, kept, memState, removed, server, up, crashes,
                   signup, runCall, ender, ending>>

(* restart.  Before #560 it wrote the slug back into Controller.states and *)
(* called Provision; since, it does nothing for a slug no longer kept.     *)
Restart ==
    /\ up /\ supervisor = "restart"
    /\ IF "restartChecksKept" \in Fixes
       THEN Reprovision
       ELSE /\ Provision
    /\ supervisor' = "idle"
    /\ UNCHANGED <<org, made, removed, up, crashes, signup, runCall, ender, ending>>

(* A slug waiting for a cell is tried again: Provision before #560,       *)
(* reprovision since.                                                      *)
Retry ==
    /\ up /\ supervisor = "wait"
    /\ IF "restartChecksKept" \in Fixes THEN Reprovision ELSE Provision
    /\ supervisor' = "idle"
    /\ UNCHANGED <<org, made, removed, up, crashes, signup, runCall, ender, ending>>

(* The cell's process for the slug stops by itself. *)
ServerDies ==
    /\ server
    /\ server' = FALSE
    /\ UNCHANGED <<org, made, dir, runtimeRow, kept, memState, removed, up, crashes,
                   signup, runCall, supervisor, ender, ending>>

-----------------------------------------------------------------------------
(* endDemo.  Controller.Remove forgets the slug, remembers which           *)
(* generation it removed, and stops the server.                            *)
Remove ==
    /\ up /\ ender = "idle" /\ org # 0
    /\ kept' = FALSE
    /\ server' = FALSE
    /\ removed' = org
    /\ ender' = "removed" /\ ending' = org
    /\ UNCHANGED <<org, made, dir, runtimeRow, memState, up, crashes, signup, runCall,
                   supervisor>>

(* Store.EndDemo: the directory, then the rows. *)
EndDemo ==
    /\ up /\ ender = "removed"
    /\ dir' = FALSE
    /\ org' = 0
    /\ runtimeRow' = FALSE
    /\ ender' = "ended"
    /\ UNCHANGED <<made, kept, memState, removed, server, up, crashes, signup, runCall,
                   supervisor, ending>>

(* Controller.Ended: the store refuses it now, so the controller forgets *)
(* the generation it removed, and the slug is free.                      *)
Ended ==
    /\ up /\ ender = "ended"
    /\ removed' = 0
    /\ ender' = "idle"
    /\ UNCHANGED <<org, made, dir, runtimeRow, kept, memState, server, up, crashes,
                   signup, runCall, supervisor, ending>>

-----------------------------------------------------------------------------
(* The front door.  A crash loses its memory and its goroutines; the store, *)
(* the directory and a server on its host stay.                             *)
Crash ==
    /\ up /\ crashes < MaxCrashes
    /\ up' = FALSE /\ crashes' = crashes + 1
    /\ kept' = FALSE /\ removed' = 0
    /\ signup' = FALSE /\ runCall' = FALSE
    /\ supervisor' = "idle"
    /\ ender' = "idle"
    /\ UNCHANGED <<org, made, dir, runtimeRow, memState, server, ending>>

(* Run: every organization with a runtime is known as Starting, then each  *)
(* is given a Provision goroutine.                                         *)
Start ==
    /\ ~up
    /\ up' = TRUE
    /\ kept' = (org # 0 /\ runtimeRow)
    /\ memState' = IF org # 0 /\ runtimeRow THEN "Starting" ELSE memState
    /\ runCall' = (org # 0)
    /\ UNCHANGED <<org, made, dir, runtimeRow, removed, server, crashes, signup, supervisor,
                   ender, ending>>

(* Every generation has been made and ended. *)
AllEnded ==
    /\ up /\ made = MaxGen /\ org = 0 /\ ender = "idle"
    /\ UNCHANGED vars

Next ==
    \/ Create \/ SignupProvision \/ RunProvision
    \/ Supervise \/ Restart \/ Retry \/ ServerDies
    \/ Remove \/ EndDemo \/ Ended
    \/ Crash \/ Start
    \/ AllEnded

Fairness ==
    /\ WF_vars(SignupProvision) /\ WF_vars(RunProvision)
    /\ WF_vars(Restart) /\ WF_vars(Retry)
    /\ WF_vars(EndDemo) /\ WF_vars(Ended) /\ WF_vars(Start)

Spec == Init /\ [][Next]_vars /\ Fairness

-----------------------------------------------------------------------------
(* What must hold. *)

(* While a demo is being ended, nothing starts its server again. *)
EndingStopsServer == ender = "removed" => ~server

(* What an organization keeps exists only while the organization does. *)
NothingOutlivesOrg ==
    /\ dir => org # 0
    /\ runtimeRow => org # 0
    /\ server => org # 0

(* An organization that takes a slug again is provisioned: what was        *)
(* removed before it does not keep it away.                                *)
NewOrgIsProvisioned ==
    \A g \in Gens :
        (org = g /\ ender = "idle" /\ removed # g) ~> (kept \/ org # g \/ ender # "idle")
=============================================================================
