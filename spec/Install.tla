------------------------------- MODULE Install -------------------------------
(***************************************************************************)
(* The OpenShell install on a person's computer, as gusto openshell        *)
(* install makes it and runs it again for a new release (internal/vmwall   *)
(* apply.go).  An install puts the programs in place, keeps the agents'    *)
(* root file system, starts the gateway for good, and last writes vm.json, *)
(* which is what gusto uses.  Any step may fail, and the install stops     *)
(* there.  Each is of one release, numbered.                               *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "clear": vm.json is taken away before the steps begin;               *)
(*  - "verify": a root file system already kept is replaced when it is not *)
(*    the release's, though its name is the same;                          *)
(*  - "stop": an install that fails stops the gateway, which nothing then  *)
(*    uses: the one it started, or the one of the install before.          *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, Releases

ASSUME Fixes \subseteq {"clear", "verify", "stop"}

VARIABLES
    programs,  \* the release of the programs in place, or 0
    image,     \* the release of the root file system kept, or 0
    record,    \* the release vm.json names, or 0 when there is none
    gateway,   \* the gateway is running, started at every login
    pc,        \* where an install is
    target     \* the release an install is of

vars == <<programs, image, record, gateway, pc, target>>

TypeOK ==
    /\ programs \in {0} \cup Releases /\ image \in {0} \cup Releases /\ record \in {0} \cup Releases
    /\ gateway \in BOOLEAN /\ pc \in {"idle", "programs", "image", "start", "record"} /\ target \in {0} \cup Releases

Init ==
    /\ programs = 0 /\ image = 0 /\ record = 0 /\ gateway = FALSE /\ pc = "idle" /\ target = 0

-----------------------------------------------------------------------------
Begin(r) ==
    /\ pc = "idle" /\ r >= target
    /\ target' = r /\ pc' = "programs"
    /\ record' = IF "clear" \in Fixes THEN 0 ELSE record
    /\ UNCHANGED <<programs, image, gateway>>

Programs ==
    /\ pc = "programs"
    /\ programs' = target /\ pc' = "image"
    /\ UNCHANGED <<image, record, gateway, target>>

(* fetchImage: without "verify", a file already there is kept. *)
Image ==
    /\ pc = "image"
    /\ image' = IF image = 0 \/ "verify" \in Fixes THEN target ELSE image
    /\ pc' = "start"
    /\ UNCHANGED <<programs, record, gateway, target>>

Start ==
    /\ pc = "start"
    /\ gateway' = TRUE /\ pc' = "record"
    /\ UNCHANGED <<programs, image, record, target>>

Record ==
    /\ pc = "record"
    /\ record' = target /\ pc' = "idle"
    /\ UNCHANGED <<programs, image, gateway, target>>

(* A step fails: the install stops there. With "stop", the gateway is stopped. *)
Fail ==
    /\ pc \in {"programs", "image", "start", "record"}
    /\ pc' = "idle"
    /\ gateway' = IF "stop" \in Fixes THEN FALSE ELSE gateway
    /\ UNCHANGED <<programs, image, record, target>>

Next == (\E r \in Releases : Begin(r)) \/ Programs \/ Image \/ Start \/ Record \/ Fail

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* What vm.json names is what is in place. *)
RecordIsTrue == record # 0 => programs = record /\ image = record

(* No gateway runs that no install gusto uses started. *)
NoIdleGateway == pc = "idle" /\ gateway => record # 0
=============================================================================
