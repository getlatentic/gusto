------------------------------ MODULE Routines ------------------------------
(***************************************************************************)
(* One agent's routines (internal/instance routine.go, internal/web        *)
(* routines.go).  A routine's slot comes; it starts a run, or skips while  *)
(* its last run is still answering, and the slot is kept as the routine's  *)
(* last.  A run's turn ends, and its news is offered to the channels until *)
(* it is settled: taken, or given up on.  A person adds a routine, which   *)
(* opens its thread and then writes routines.json; the write may fail.    *)
(* routines.json may be damaged, and the page and the runner read it.      *)
(* Writes are whole or not at all (StateFile), so damage is from outside.  *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "owe": a slot that takes the last place of a run whose news is not   *)
(*    settled keeps that run among the routine's owed news;                *)
(*  - "undo": an add whose write fails closes the thread it opened;        *)
(*  - "surface": a damaged routines.json is reported, not read as none.    *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, MaxRuns, MaxSlots

ASSUME Fixes \subseteq {"owe", "undo", "surface"}

VARIABLES
    runs,     \* how many runs have started; run n is numbered n
    running,  \* the run whose turn is answering, or 0
    ended,    \* the runs whose turn has ended
    last,     \* the latest slot's run, or 0 for a skip
    owed,     \* earlier runs whose news is kept until it is settled
    settled,  \* the runs whose news is taken or given up on
    slots,
    file,     \* routines.json: whole or damaged
    shown,    \* what the last read gave: the routines, none, or damaged
    kept,     \* how many routines the file holds
    threads,  \* how many routine threads there are
    adding    \* an add has opened its thread and not written the routine

vars == <<runs, running, ended, last, owed, settled, slots, file, shown, kept, threads, adding>>

Runs == 1..MaxRuns

TypeOK ==
    /\ runs \in 0..MaxRuns /\ running \in 0..MaxRuns /\ last \in 0..MaxRuns
    /\ ended \subseteq Runs /\ owed \subseteq Runs /\ settled \subseteq Runs
    /\ slots \in 0..MaxSlots
    /\ file \in {"whole", "damaged"} /\ shown \in {"routines", "none", "damaged"}
    /\ kept \in 1..2 /\ threads \in 1..2 /\ adding \in BOOLEAN

Init ==
    /\ runs = 0 /\ running = 0 /\ ended = {} /\ last = 0 /\ owed = {} /\ settled = {}
    /\ slots = 0 /\ file = "whole" /\ shown = "routines"
    /\ kept = 1 /\ threads = 1 /\ adding = FALSE

-----------------------------------------------------------------------------
(* RoutineRan keeps the slot as the routine's last. *)
Keep(r) ==
    /\ last' = r
    /\ owed' = IF "owe" \in Fixes /\ last # 0 /\ last \notin settled THEN owed \cup {last} ELSE owed

(* startRoutine: a run, or a skip while the last one is answering. *)
Slot ==
    /\ file = "whole" /\ slots < MaxSlots
    /\ slots' = slots + 1
    /\ IF running = 0 /\ runs < MaxRuns
       THEN /\ runs' = runs + 1 /\ running' = runs + 1 /\ Keep(runs + 1)
       ELSE /\ Keep(0) /\ UNCHANGED <<runs, running>>
    /\ UNCHANGED <<ended, settled, file, shown, kept, threads, adding>>

End ==
    /\ running # 0
    /\ ended' = ended \cup {running} /\ running' = 0
    /\ UNCHANGED <<runs, last, owed, settled, slots, file, shown, kept, threads, adding>>

(* tellRoutines settles the news of an ended run it can find. *)
Tell(n) ==
    /\ file = "whole"
    /\ n \in (({last} \cup owed) \cap ended) \ settled
    /\ settled' = settled \cup {n} /\ owed' = owed \ {n}
    /\ UNCHANGED <<runs, running, ended, last, slots, file, shown, kept, threads, adding>>

-----------------------------------------------------------------------------
(* AddRoutine opens the routine's thread, then writes routines.json. *)
Add ==
    /\ file = "whole" /\ ~adding /\ kept = 1
    /\ threads' = threads + 1 /\ adding' = TRUE
    /\ UNCHANGED <<runs, running, ended, last, owed, settled, slots, file, shown, kept>>

Written ==
    /\ adding
    /\ kept' = kept + 1 /\ adding' = FALSE
    /\ UNCHANGED <<runs, running, ended, last, owed, settled, slots, file, shown, threads>>

NotWritten ==
    /\ adding
    /\ adding' = FALSE
    /\ threads' = IF "undo" \in Fixes THEN threads - 1 ELSE threads
    /\ UNCHANGED <<runs, running, ended, last, owed, settled, slots, file, shown, kept>>

Damage ==
    /\ file = "whole" /\ ~adding
    /\ file' = "damaged"
    /\ UNCHANGED <<runs, running, ended, last, owed, settled, slots, shown, kept, threads, adding>>

(* Routines, as the page and the runner read them. *)
Read ==
    /\ shown' = IF file = "whole" THEN "routines"
                ELSE IF "surface" \in Fixes THEN "damaged" ELSE "none"
    /\ UNCHANGED <<runs, running, ended, last, owed, settled, slots, file, kept, threads, adding>>

Next ==
    \/ Slot \/ End \/ \E n \in Runs : Tell(n)
    \/ Add \/ Written \/ NotWritten \/ Damage \/ Read

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* Every run's news is settled, or still kept to be told. *)
NewsKept == \A n \in 1..runs : n \in settled \/ n = last \/ n \in owed

(* Every routine thread belongs to a routine, once no add is part way. *)
NoOrphanThread == ~adding => threads = kept

(* A read that gives no routines is one of a file that holds none. *)
NoneMeansNone == shown = "none" => kept = 0
=============================================================================
