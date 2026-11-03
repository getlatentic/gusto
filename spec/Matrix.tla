------------------------------- MODULE Matrix --------------------------------
(***************************************************************************)
(* One cell of a benchmark matrix, as gusto matrix measures it            *)
(* (internal/matrix matrix.go, prepare.go).  A measure readies the cell's  *)
(* directory and runs it when it is due: one with no verdict, or one asked *)
(* for again.  A used directory is moved aside and made again first.  A    *)
(* run may leave a verdict, and may fail after it.  The measure may stop   *)
(* at any step, and is run again.  The cell's time is which run it was     *)
(* measured around.                                                        *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "remake": a measure makes again a cell whose directory a stop left   *)
(*    without its job.json;                                                *)
(*  - "time": a cell's time is cleared before its run and set whenever the *)
(*    run leaves a verdict.                                                *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, MaxRuns, MaxStops

ASSUME Fixes \subseteq {"remake", "time"}

VARIABLES
    dir,      \* the cell's directory: made, used, or missing
    run,      \* the number of the run the directory holds, or will
    verdict,  \* that run left a verdict
    timed,    \* the run the cell's time was measured around, or 0
    pc,       \* where the measure is
    stuck,    \* a run was started in a directory that has no job.json
    stops

vars == <<dir, run, verdict, timed, pc, stuck, stops>>

TypeOK ==
    /\ dir \in {"made", "used", "missing"} /\ run \in 0..MaxRuns /\ verdict \in BOOLEAN
    /\ timed \in 0..MaxRuns /\ pc \in {"idle", "clean", "make", "run", "stopped"}
    /\ stuck \in BOOLEAN /\ stops \in 0..MaxStops

Init ==
    /\ dir = "made" /\ run = 1 /\ verdict = FALSE /\ timed = 0
    /\ pc = "idle" /\ stuck = FALSE /\ stops = 0

-----------------------------------------------------------------------------
(* prepared, then due: a measure begins, and the cell is run when it has no *)
(* verdict or is asked for again.                                           *)
Measure ==
    /\ pc \in {"idle", "stopped"} /\ run < MaxRuns
    /\ dir' = IF dir = "missing" /\ "remake" \in Fixes THEN "made" ELSE dir
    /\ pc' = "clean"
    /\ UNCHANGED <<run, verdict, timed, stuck, stops>>

(* clean moves a used directory aside, *)
Retire ==
    /\ pc = "clean"
    /\ IF dir = "used"
       THEN dir' = "missing" /\ run' = run + 1 /\ verdict' = FALSE /\ pc' = "make"
       ELSE UNCHANGED <<dir, run, verdict>> /\ pc' = "run"
    /\ UNCHANGED <<timed, stuck, stops>>

(* and makes it again. *)
Make ==
    /\ pc = "make"
    /\ dir' = "made" /\ pc' = "run"
    /\ UNCHANGED <<run, verdict, timed, stuck, stops>>

(* runOne: with "time" the time is cleared first; a run in a missing        *)
(* directory fails at job.Read.                                             *)
Run ==
    /\ pc = "run"
    /\ stuck' = (stuck \/ dir = "missing")
    /\ \E left \in BOOLEAN, failed \in BOOLEAN :
         /\ verdict' = (left /\ dir # "missing")
         /\ timed' = IF verdict' /\ ("time" \in Fixes \/ ~failed) THEN run
                     ELSE IF "time" \in Fixes THEN 0 ELSE timed
    /\ dir' = IF dir = "missing" THEN dir ELSE "used"
    /\ pc' = "idle"
    /\ UNCHANGED <<run, stops>>

Stop ==
    /\ pc \in {"clean", "make", "run"} /\ stops < MaxStops
    /\ pc' = "stopped" /\ stops' = stops + 1
    /\ UNCHANGED <<dir, run, verdict, timed, stuck>>

Next == Measure \/ Retire \/ Make \/ Run \/ Stop

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* No run is started in a directory that has no job.json. *)
NeverStuck == ~stuck

(* A cell whose run left a verdict is timed around that run. *)
TimeIsTheRun == pc = "idle" /\ verdict => timed = run
=============================================================================
