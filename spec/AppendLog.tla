----------------------------- MODULE AppendLog -----------------------------
(***************************************************************************)
(* The change log (internal/changelog): numbered, hash-chained lines that *)
(* Append writes one at a time, under its lock, each synced before it      *)
(* returns.  Open checks every line and refuses a log that does not hold   *)
(* together, since what would follow a cut or a changed line would look   *)
(* as sound as what came before.                                           *)
(*                                                                         *)
(* A write may be short (a full disk), and a sync may fail.  The kernel   *)
(* may write a line to disk part way.  A host crash leaves what is on     *)
(* disk; a process crash leaves what processes see.                        *)
(*                                                                         *)
(* Fixes is which fixes the code has: "rollback" is that Append, when its *)
(* write or sync fails, cuts the file back to the last line it knows was  *)
(* written whole; "repairTail" is that Open removes a last line with no   *)
(* end, which no Append ever said was done, before it checks the log.     *)
(***************************************************************************)
EXTENDS Naturals, Sequences

CONSTANTS
    Fixes,        \* a subset of {"rollback", "repairTail"}
    MaxEvents,    \* how many events are appended
    MaxCrashes,   \* how many crashes, of either kind
    MaxFailures   \* how many writes or syncs fail

ASSUME Fixes \subseteq {"rollback", "repairTail"}

(* A line: the number it carries, and whether all of it, ending in a      *)
(* newline, is there.                                                      *)
Lines == [seq: 1..MaxEvents, whole: BOOLEAN]

VARIABLES
    seen,       \* the file as processes see it: a sequence of lines
    disk,       \* the file on disk
    known,      \* how many lines Append knows are written whole
    next,       \* the number Append gives the next event
    appender,   \* where Append is
    acked,      \* the events Append said were written
    up,         \* the server runs
    refused,    \* Open refused the log, so the server does not start
    cutWhole,   \* Open removed a whole line: a repair that would hide a cut
    crashes,
    failures

vars == <<seen, disk, known, next, appender, acked, up, refused, cutWhole, crashes, failures>>

TypeOK ==
    /\ seen \in Seq(Lines) /\ disk \in Seq(Lines)
    /\ known \in Nat /\ next \in 1..MaxEvents + 1
    /\ appender \in {"idle", "written", "failed"}
    /\ acked \subseteq 1..MaxEvents
    /\ up \in BOOLEAN /\ refused \in BOOLEAN /\ cutWhole \in BOOLEAN
    /\ crashes \in 0..MaxCrashes /\ failures \in 0..MaxFailures

Init ==
    /\ seen = <<>> /\ disk = <<>> /\ known = 0 /\ next = 1
    /\ appender = "idle" /\ acked = {}
    /\ up = TRUE /\ refused = FALSE /\ cutWhole = FALSE
    /\ crashes = 0 /\ failures = 0

Prefix(s, n) == SubSeq(s, 1, n)

-----------------------------------------------------------------------------
(* Append writes the line, whole, or short when the disk is full.  A short *)
(* write fails.                                                            *)
Write ==
    /\ up /\ appender = "idle" /\ next <= MaxEvents
    /\ \/ /\ seen' = Append(seen, [seq |-> next, whole |-> TRUE])
          /\ appender' = "written"
          /\ UNCHANGED failures
       \/ /\ failures < MaxFailures
          /\ seen' = Append(seen, [seq |-> next, whole |-> FALSE])
          /\ appender' = "failed"
          /\ failures' = failures + 1
    /\ UNCHANGED <<disk, known, next, acked, up, refused, cutWhole, crashes>>

(* The sync puts the file on disk, and Append says the event is written. *)
Sync ==
    /\ up /\ appender = "written"
    /\ disk' = seen
    /\ known' = Len(seen)
    /\ acked' = acked \cup {next}
    /\ next' = next + 1
    /\ appender' = "idle"
    /\ UNCHANGED <<seen, up, refused, cutWhole, crashes, failures>>

(* The sync fails: the line may or may not be on disk. *)
SyncFails ==
    /\ up /\ appender = "written" /\ failures < MaxFailures
    /\ failures' = failures + 1
    /\ appender' = "failed"
    /\ UNCHANGED <<seen, disk, known, next, acked, up, refused, cutWhole, crashes>>

(* Append returns the error.  With rollback the file is cut back to the  *)
(* lines it knows, and that is synced; without it the bytes stay, and the *)
(* next event is written after them with the same number.                 *)
Failed ==
    /\ up /\ appender = "failed"
    /\ IF "rollback" \in Fixes
       THEN /\ seen' = Prefix(seen, known)
            /\ disk' = Prefix(seen, known)
       ELSE UNCHANGED <<seen, disk>>
    /\ appender' = "idle"
    /\ UNCHANGED <<known, next, acked, up, refused, cutWhole, crashes, failures>>

-----------------------------------------------------------------------------
(* The kernel writes the file to disk on its own, perhaps only part of the *)
(* last line.                                                              *)
Flush ==
    /\ disk # seen /\ Len(seen) > 0
    /\ \/ disk' = seen
       \/ disk' = [seen EXCEPT ![Len(seen)] = [@ EXCEPT !.whole = FALSE]]
    /\ UNCHANGED <<seen, known, next, appender, acked, up, refused, cutWhole, crashes, failures>>

ProcessCrash ==
    /\ up /\ crashes < MaxCrashes
    /\ crashes' = crashes + 1
    /\ up' = FALSE /\ appender' = "idle"
    /\ UNCHANGED <<seen, disk, known, next, acked, refused, cutWhole, failures>>

HostCrash ==
    /\ up /\ crashes < MaxCrashes
    /\ crashes' = crashes + 1
    /\ seen' = disk
    /\ up' = FALSE /\ appender' = "idle"
    /\ UNCHANGED <<disk, known, next, acked, refused, cutWhole, failures>>

(* Every line whole, numbered 1, 2, 3 in order: what Verify takes. *)
Sound(s) == \A i \in 1..Len(s) : s[i].whole /\ s[i].seq = i

(* Open, as the server starts.  With repairTail a last line that is not   *)
(* whole is removed first.                                                 *)
Open ==
    /\ ~up /\ ~refused
    /\ LET last == Len(seen)
           torn == last > 0 /\ ~seen[last].whole
           kept == IF "repairTail" \in Fixes /\ torn THEN Prefix(seen, last - 1) ELSE seen
       IN /\ seen' = kept
          /\ disk' = IF kept # seen THEN kept ELSE disk
          /\ cutWhole' = (cutWhole \/ \E i \in Len(kept) + 1..last : seen[i].whole)
          /\ IF Sound(kept)
             THEN /\ up' = TRUE /\ known' = Len(kept) /\ next' = Len(kept) + 1
                  /\ UNCHANGED refused
             ELSE /\ refused' = TRUE
                  /\ UNCHANGED <<up, known, next>>
    /\ UNCHANGED <<appender, acked, crashes, failures>>

Next ==
    \/ Write \/ Sync \/ SyncFails \/ Failed
    \/ Flush \/ ProcessCrash \/ HostCrash \/ Open

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* A crash or a failed write never stops the server starting: only a log *)
(* that was cut or changed does, and nothing here cuts or changes one.   *)
AlwaysStarts == ~refused

(* Every event Append said was written is in the log, at its number. *)
NothingDoneIsLost ==
    up => \A n \in acked : n <= Len(seen) /\ seen[n].seq = n /\ seen[n].whole

(* Open's repair removes only a line no Append finished, never a whole one. *)
RepairHidesNothing == ~cutWhole
=============================================================================
