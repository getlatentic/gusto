------------------------------- MODULE Record --------------------------------
(***************************************************************************)
(* A room turn's record, as Decide puts what was done with its changes     *)
(* under its signature (internal/record run.go, record.go, seal.go).       *)
(* The run ends with out/ hashed into manifest.json and the manifest       *)
(* signed.  The turn's changes are then merged or refused, and Decide      *)
(* writes decision.json, makes the manifest again and signs it again.  A   *)
(* file in out/ may be changed after the run was signed.  The server may   *)
(* crash at any step, and when it starts again it finishes what it can.    *)
(* A record is out/ as one of: what the run left, that and its decision,   *)
(* or that changed; the manifest is out/ as it hashed it; the signature    *)
(* is the manifest it was made over.                                       *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "pending": the new manifest and its signature are made first and     *)
(*    kept beside out/, then put in place, and a start finishes them;      *)
(*  - "intact": a record whose signature does not hold for out/ is not     *)
(*    signed again.                                                        *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, MaxCrashes

ASSUME Fixes \subseteq {"pending", "intact"}

Pending == "pending" \in Fixes

VARIABLES
    files,     \* out/: ran, decided, or changed
    manifest,  \* what the manifest hashed
    sig,       \* the manifest the signature was made over, or none
    pc,        \* where Decide is
    kept,      \* a pending seal is beside out/
    crashes

vars == <<files, manifest, sig, pc, kept, crashes>>

TypeOK ==
    /\ files \in {"ran", "decided", "changed"}
    /\ manifest \in {"ran", "decided", "changed"}
    /\ sig \in {"none", "ran", "decided", "changed"}
    /\ pc \in {"ready", "decision", "manifest", "signature", "clear", "crashed", "done"}
    /\ kept \in BOOLEAN /\ crashes \in 0..MaxCrashes

Init ==
    /\ files = "ran" /\ manifest = "ran" /\ sig = "ran"
    /\ pc = "ready" /\ kept = FALSE /\ crashes = 0

(* What out/ is once the decision is in it: a changed record stays changed. *)
Decided(f) == IF f = "changed" THEN "changed" ELSE "decided"

-----------------------------------------------------------------------------
(* Somebody changes a file in out/ after the run was signed. *)
Change ==
    /\ pc = "ready" /\ files = "ran"
    /\ files' = "changed"
    /\ UNCHANGED <<manifest, sig, pc, kept, crashes>>

(* Decide begins: with "intact" it refuses a record its signature does not *)
(* hold for; with "pending" it keeps the seal it is about to put in place. *)
Begin ==
    /\ pc = "ready"
    /\ IF "intact" \in Fixes /\ (sig # manifest \/ manifest # files)
       THEN pc' = "done" /\ UNCHANGED kept
       ELSE pc' = "decision" /\ kept' = Pending
    /\ UNCHANGED <<files, manifest, sig, crashes>>

WriteDecision ==
    /\ pc = "decision"
    /\ files' = Decided(files) /\ pc' = "manifest"
    /\ UNCHANGED <<manifest, sig, kept, crashes>>

(* Without "pending", Write takes the old signature away with the old manifest. *)
WriteManifest ==
    /\ pc = "manifest"
    /\ manifest' = files
    /\ sig' = IF Pending THEN sig ELSE "none"
    /\ pc' = "signature"
    /\ UNCHANGED <<files, kept, crashes>>

WriteSignature ==
    /\ pc = "signature"
    /\ sig' = manifest /\ pc' = IF Pending THEN "clear" ELSE "done"
    /\ UNCHANGED <<files, manifest, kept, crashes>>

Clear ==
    /\ pc = "clear"
    /\ kept' = FALSE /\ pc' = "done"
    /\ UNCHANGED <<files, manifest, sig, crashes>>

Crash ==
    /\ pc \in {"decision", "manifest", "signature", "clear"} /\ crashes < MaxCrashes
    /\ pc' = "crashed" /\ crashes' = crashes + 1
    /\ UNCHANGED <<files, manifest, sig, kept>>

(* The server starts again: a pending seal is put in place from the start. *)
Restart ==
    /\ pc = "crashed"
    /\ pc' = IF kept THEN "decision" ELSE "done"
    /\ UNCHANGED <<files, manifest, sig, kept, crashes>>

Next ==
    \/ Change \/ Begin \/ WriteDecision \/ WriteManifest \/ WriteSignature \/ Clear
    \/ Crash \/ Restart

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* A signed record that Decide is done with is signed over its manifest. *)
StaysSigned == pc = "done" => sig # "none" /\ sig = manifest

(* The signature never vouches for a file changed after the run was signed. *)
NothingChangedIsSigned == sig # "changed"
=============================================================================
