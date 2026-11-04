------------------------------- MODULE Formats -------------------------------
(***************************************************************************)
(* The format of what gusto keeps, and how it is moved on and back.        *)
(*                                                                         *)
(* A data directory (internal/datadir) has data in some format and a stamp *)
(* that says which.  One made before stamps has data at format 1 and no    *)
(* stamp.  An upgrade runs each step up from the format it reads; a        *)
(* rollback runs each step down to the format it is asked for.  A stamp is *)
(* written to a scratch file and renamed over the old one, and two         *)
(* releases may stamp at once.                                             *)
(*                                                                         *)
(* A database (internal/dbkit) has had some of a component's migrations,   *)
(* each kept in its version table.  A migration can be added later with a  *)
(* number below one the database has had: a branch merged after another.  *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "format": the rollback reads a directory's format as the upgrade     *)
(*    does, so data with no stamp is format 1 to both;                     *)
(*  - "scratch": each stamp is written to a scratch file of its own;       *)
(*  - "applied": a database is migrated by the migrations it has had, and  *)
(*    one that passed over a migration numbered below its newest is        *)
(*    refused.                                                             *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets

CONSTANTS Fixes, Current, Writers

ASSUME Fixes \subseteq {"format", "scratch", "applied"}

VARIABLES
    data,      \* the format the directory's data is in
    stamp,     \* the format its stamp says, or 0 when it has none
    scratch,   \* the scratch files written and not yet renamed
    writing,   \* the writers between writing and renaming
    undone,    \* a step was undone on data not in the format it undoes
    lost,      \* a stamp's rename found no scratch file
    files,     \* the migrations this release has
    applied,   \* the migrations the database has had
    migrated   \* the last migration said the database is up to date

vars == <<data, stamp, scratch, writing, undone, lost, files, applied, migrated>>

Formats == 1..Current

Own(w) == IF "scratch" \in Fixes THEN w ELSE "shared"

TypeOK ==
    /\ data \in Formats /\ stamp \in {0} \cup Formats
    /\ scratch \subseteq Writers \cup {"shared"} /\ writing \subseteq Writers
    /\ undone \in BOOLEAN /\ lost \in BOOLEAN
    /\ files \subseteq 1..3 /\ applied \subseteq 1..3 /\ migrated \in BOOLEAN

Init ==
    /\ data = 1 /\ stamp = 0 /\ scratch = {} /\ writing = {}
    /\ undone = FALSE /\ lost = FALSE
    /\ files = {1, 3} /\ applied = {} /\ migrated = FALSE

-----------------------------------------------------------------------------
(* formatOf, as Ensure and Plan read it. *)
Upgrading == IF stamp = 0 THEN 1 ELSE stamp
(* What undoing read before "format". *)
RollingBack == IF "format" \in Fixes THEN Upgrading ELSE IF stamp = 0 THEN Current ELSE stamp

Upgrade ==
    /\ Upgrading < Current
    /\ data' = Upgrading + 1 /\ stamp' = Upgrading + 1
    /\ UNCHANGED <<scratch, writing, undone, lost, files, applied, migrated>>

Rollback(to) ==
    /\ to <= RollingBack
    /\ undone' = (undone \/ (to < RollingBack /\ data # RollingBack))
    /\ data' = IF to < RollingBack THEN to ELSE data
    /\ stamp' = to
    /\ UNCHANGED <<scratch, writing, lost, files, applied, migrated>>

(* write: the scratch file, then the rename. *)
WriteScratch(w) ==
    /\ w \notin writing
    /\ scratch' = scratch \cup {Own(w)} /\ writing' = writing \cup {w}
    /\ UNCHANGED <<data, stamp, undone, lost, files, applied, migrated>>

Rename(w) ==
    /\ w \in writing
    /\ lost' = (lost \/ Own(w) \notin scratch)
    /\ scratch' = scratch \ {Own(w)} /\ writing' = writing \ {w}
    /\ stamp' = IF Own(w) \in scratch THEN data ELSE stamp
    /\ UNCHANGED <<data, undone, files, applied>> /\ UNCHANGED migrated

-----------------------------------------------------------------------------
Newest(set) == IF set = {} THEN 0 ELSE CHOOSE n \in set : \A m \in set : m <= n

(* Migrate.  Without "applied" it applies what is numbered above the newest *)
(* the database has had.                                                    *)
Migrate ==
    IF "applied" \in Fixes
    THEN IF \E n \in files : n < Newest(applied) /\ n \notin applied
         THEN migrated' = FALSE /\ UNCHANGED applied
         ELSE applied' = applied \cup files /\ migrated' = TRUE
    ELSE /\ applied' = applied \cup {n \in files : n > Newest(applied)}
         /\ migrated' = TRUE

MigrateStep ==
    /\ Migrate
    /\ UNCHANGED <<data, stamp, scratch, writing, undone, lost, files>>

(* A branch adds migration 2 after 3 was applied. *)
AddLate ==
    /\ 2 \notin files
    /\ files' = files \cup {2} /\ migrated' = FALSE
    /\ UNCHANGED <<data, stamp, scratch, writing, undone, lost, applied>>

Next ==
    \/ Upgrade \/ \E to \in Formats : Rollback(to)
    \/ \E w \in Writers : WriteScratch(w) \/ Rename(w)
    \/ MigrateStep \/ AddLate

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* No step is undone on data in another format. *)
UndoneIsUpgraded == ~undone

(* Every stamp written is put in place. *)
StampsLand == ~lost

(* A database said to be up to date has had every migration. *)
UpToDateIsWhole == migrated => files \subseteq applied
=============================================================================
