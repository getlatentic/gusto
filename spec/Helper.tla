------------------------------- MODULE Helper --------------------------------
(***************************************************************************)
(* The desktop helper on one computer (internal/helper daemon.go,          *)
(* mirror/engine.go).  Helpers start: each looks for one already running   *)
(* and, finding none, says where it is and serves.  Engines keep a pair of *)
(* folders the same, the helper's and a sync beside it: each run reads the *)
(* folders' record, works, and writes the record again.                    *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "lock": a helper holds a lock before it looks and while it runs;     *)
(*  - "hold": an engine's run holds the record's lock and reads the record *)
(*    again under it, instead of using what it read when it was made.      *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, Helpers, Engines, MaxWrites

ASSUME Fixes \subseteq {"lock", "hold"}

VARIABLES
    phase,    \* each helper: off, looked, serving, or refused
    locked,   \* the helper holding the lock, or none
    record,   \* how many runs the folders' record holds
    seen,     \* each engine: how many runs the record held when it read it
    lost      \* a run wrote the record over one it had not read

vars == <<phase, locked, record, seen, lost>>

TypeOK ==
    /\ phase \in [Helpers -> {"off", "looked", "serving", "refused"}]
    /\ locked \in Helpers \cup {"none"}
    /\ record \in 0..MaxWrites /\ seen \in [Engines -> 0..MaxWrites] /\ lost \in BOOLEAN

Init ==
    /\ phase = [h \in Helpers |-> "off"] /\ locked = "none"
    /\ record = 0 /\ seen = [e \in Engines |-> 0] /\ lost = FALSE

Serving(h) == phase[h] = "serving"

-----------------------------------------------------------------------------
(* Run: with "lock" the lock is taken first, and a helper that cannot take *)
(* it refuses.  Then Discover and Ask: a helper that is serving refuses it. *)
Look(h) ==
    /\ phase[h] = "off"
    /\ IF "lock" \in Fixes
       THEN IF locked = "none"
            THEN locked' = h /\ phase' = [phase EXCEPT ![h] = "looked"]
            ELSE phase' = [phase EXCEPT ![h] = "refused"] /\ UNCHANGED locked
       ELSE /\ phase' = [phase EXCEPT ![h] = IF \E g \in Helpers : Serving(g) THEN "refused" ELSE "looked"]
            /\ UNCHANGED locked
    /\ UNCHANGED <<record, seen, lost>>

(* publish, then serve. *)
Serve(h) ==
    /\ phase[h] = "looked"
    /\ phase' = [phase EXCEPT ![h] = "serving"]
    /\ UNCHANGED <<locked, record, seen, lost>>

(* Once: with "hold" the record is read again under its lock. *)
Once(e) ==
    /\ record < MaxWrites
    /\ LET read == IF "hold" \in Fixes THEN record ELSE seen[e]
       IN /\ lost' = (lost \/ read < record)
          /\ record' = record + 1
          /\ seen' = [seen EXCEPT ![e] = record + 1]
    /\ UNCHANGED <<phase, locked>>

Next == \E h \in Helpers : Look(h) \/ Serve(h)
        \/ \E e \in Engines : Once(e)

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* One helper serves at a time. *)
OneHelper == \A g, h \in Helpers : Serving(g) /\ Serving(h) => g = h

(* No run writes the folders' record over a run it did not read. *)
NothingLost == ~lost
=============================================================================
