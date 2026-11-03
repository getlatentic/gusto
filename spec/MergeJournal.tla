---------------------------- MODULE MergeJournal ----------------------------
(***************************************************************************)
(* One merge into a room's folder (internal/roomwork/apply.go): each file *)
(* is staged, a journal is written, each step keeps the file it replaces  *)
(* in a backup and renames the staged one into place, and removing the   *)
(* journal makes the merge final.  A crash leaves the journal, and         *)
(* recoverMerge undoes the steps that ran.  Putting a file back may fail. *)
(*                                                                         *)
(* Every file the merge replaces was in the folder before it.            *)
(*                                                                         *)
(* Fixes is which fixes the code has: "keepUntilUndone" is that           *)
(* recoverMerge removes the backups and the journal only when the undo    *)
(* put every file back, as commit already does, so a later start tries   *)
(* again; "undoTwice" is that the journal says which files were there     *)
(* before, so an undo run again does not remove a file it put back.      *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Fixes,       \* a subset of {"keepUntilUndone", "undoTwice"}
    Files,       \* the files the merge replaces
    MaxCrashes,
    MaxFailures  \* how many times putting a file back fails

ASSUME Fixes \subseteq {"keepUntilUndone", "undoTwice"}

VARIABLES
    dest,      \* each file in the folder: the old version, the new, or gone
    backup,    \* whether each file's backup holds the old version
    staged,    \* whether each file's staged copy is still there
    journal,   \* the journal is there
    final,     \* the journal was removed: the merge is final
    merger,    \* where the merge is
    crashes,
    failures

vars == <<dest, backup, staged, journal, final, merger, crashes, failures>>

TypeOK ==
    /\ dest \in [Files -> {"old", "new", "gone"}] /\ backup \in [Files -> BOOLEAN]
    /\ staged \in [Files -> BOOLEAN] /\ journal \in BOOLEAN /\ final \in BOOLEAN
    /\ merger \in {"staging", "journaled", "done", "down"}
    /\ crashes \in 0..MaxCrashes /\ failures \in 0..MaxFailures

Init ==
    /\ dest = [f \in Files |-> "old"] /\ backup = [f \in Files |-> FALSE]
    /\ staged = [f \in Files |-> FALSE] /\ journal = FALSE /\ final = FALSE
    /\ merger = "staging" /\ crashes = 0 /\ failures = 0

(* stage, then the journal (written whole). *)
Stage ==
    /\ merger = "staging"
    /\ staged' = [f \in Files |-> TRUE]
    /\ journal' = TRUE
    /\ merger' = "journaled"
    /\ UNCHANGED <<dest, backup, final, crashes, failures>>

(* step.do: keep the file in its backup, then rename the staged one over it. *)
Keep(f) ==
    /\ merger = "journaled" /\ staged[f] /\ ~backup[f]
    /\ backup' = [backup EXCEPT ![f] = TRUE]
    /\ UNCHANGED <<dest, staged, journal, final, merger, crashes, failures>>

Place(f) ==
    /\ merger = "journaled" /\ staged[f] /\ backup[f]
    /\ dest' = [dest EXCEPT ![f] = "new"]
    /\ staged' = [staged EXCEPT ![f] = FALSE]
    /\ UNCHANGED <<backup, journal, final, merger, crashes, failures>>

(* Removing the journal makes the merge final; forget then removes the backups. *)
Finish ==
    /\ merger = "journaled" /\ \A f \in Files : ~staged[f]
    /\ journal' = FALSE /\ final' = TRUE
    /\ backup' = [f \in Files |-> FALSE]
    /\ merger' = "done"
    /\ UNCHANGED <<dest, staged, crashes, failures>>

Crash ==
    /\ merger \in {"staging", "journaled"} /\ crashes < MaxCrashes
    /\ crashes' = crashes + 1
    /\ merger' = "down"
    /\ UNCHANGED <<dest, backup, staged, journal, final, failures>>

(* recoverMerge, when the room is next used: undo, step by step.  A step *)
(* whose staged copy is still there did not run.  One whose backup is     *)
(* there is put back from it, which may fail.  One with neither is taken  *)
(* for a file the merge made, and removed: unless undoTwice says the file *)
(* was there before, when it was put back already.  Then forget removes   *)
(* the backups, the stage and the journal: always, or with                *)
(* keepUntilUndone only when the undo put all back.                       *)
Recover ==
    /\ merger = "down" /\ journal
    /\ \E failing \in SUBSET {f \in Files : ~staged[f] /\ backup[f]} :
         /\ failures + (IF failing = {} THEN 0 ELSE 1) <= MaxFailures
         /\ failures' = failures + (IF failing = {} THEN 0 ELSE 1)
         /\ LET back == {f \in Files : ~staged[f] /\ backup[f]} \ failing
                removed == IF "undoTwice" \in Fixes THEN {} ELSE {f \in Files : ~staged[f] /\ ~backup[f]}
                undone == failing = {}
                forgets == undone \/ "keepUntilUndone" \notin Fixes
            IN /\ dest' = [f \in Files |-> CASE f \in back -> "old" [] f \in removed -> "gone" [] OTHER -> dest[f]]
               /\ backup' = IF forgets THEN [f \in Files |-> FALSE] ELSE [f \in Files |-> backup[f] /\ f \notin back]
               /\ staged' = IF forgets THEN [f \in Files |-> FALSE] ELSE staged
               /\ journal' = ~forgets
               /\ merger' = IF undone THEN "staging" ELSE "down"
    /\ UNCHANGED <<final, crashes>>

Next ==
    \/ Stage \/ (\E f \in Files : Keep(f) \/ Place(f)) \/ Finish
    \/ Crash \/ Recover

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* A merge is all or nothing: until it is final, each file it replaced is *)
(* old in the folder or kept in its backup, so the folder can be put back. *)
AllOrNothing == final \/ \A f \in Files : dest[f] = "old" \/ (dest[f] = "new" /\ backup[f])
=============================================================================
