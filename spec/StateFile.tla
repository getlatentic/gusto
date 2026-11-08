----------------------------- MODULE StateFile -----------------------------
(***************************************************************************)
(* One state file that gusto replaces with each new version, as a task's  *)
(* state.json or a room's thread file, through a process crash or a host  *)
(* crash.  The file system is two layers: what processes see (the page    *)
(* cache) and what a host crash leaves (the disk).  The kernel writes a   *)
(* file's data and a directory's names to disk on its own, in any order;  *)
(* fsync writes one file's data, and fsync of the directory its names.    *)
(*                                                                         *)
(* Protocol is how a version is written:                                   *)
(*  - "inPlace": truncate the file and write it, as os.WriteFile does;    *)
(*  - "rename": write a temporary file and rename it over the file, as    *)
(*    atomicfile.Write did;                                                *)
(*  - "renameSynced": the same, with the temporary file fsynced before    *)
(*    the rename;                                                          *)
(*  - "renameSyncedDir": the same, with the directory fsynced after the   *)
(*    rename, before the write is said to be done.                         *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Protocol,      \* one of the four above
    MaxVersions,   \* how many versions are written
    MaxCrashes     \* how many crashes, of either kind

ASSUME Protocol \in {"inPlace", "rename", "renameSynced", "renameSyncedDir"}

Inodes == 0..MaxVersions
NoInode == MaxVersions + 1

(* What an inode holds: nothing yet, half of a version, or all of one. *)
Contents == [kind: {"empty", "half", "whole"}, version: 0..MaxVersions]
Empty == [kind |-> "empty", version |-> 0]
Half(v) == [kind |-> "half", version |-> v]
Whole(v) == [kind |-> "whole", version |-> v]

VARIABLES
    seenName,   \* the inode each name points to, as processes see it
    diskName,   \* the inode each name points to on disk
    seenData,   \* each inode's contents, as processes see them
    diskData,   \* each inode's contents on disk
    writer,     \* where the writer is
    writing,    \* the version it is writing
    acked,      \* the newest version the writer said was done
    crashes

vars == <<seenName, diskName, seenData, diskData, writer, writing, acked, crashes>>

Names == {"file", "temp"}

TypeOK ==
    /\ seenName \in [Names -> Inodes \cup {NoInode}]
    /\ diskName \in [Names -> Inodes \cup {NoInode}]
    /\ seenData \in [Inodes -> Contents]
    /\ diskData \in [Inodes -> Contents]
    /\ writer \in {"idle", "truncated", "half", "made", "written", "synced", "renamed"}
    /\ writing \in 0..MaxVersions
    /\ acked \in 0..MaxVersions
    /\ crashes \in 0..MaxCrashes

(* The file starts holding version 0, on disk. *)
Init ==
    /\ seenName = [n \in Names |-> IF n = "file" THEN 0 ELSE NoInode]
    /\ diskName = seenName
    /\ seenData = [i \in Inodes |-> IF i = 0 THEN Whole(0) ELSE Empty]
    /\ diskData = seenData
    /\ writer = "idle" /\ writing = 0 /\ acked = 0 /\ crashes = 0

Done(v) ==
    /\ writer' = "idle" /\ acked' = v
    /\ UNCHANGED <<writing, crashes>>

-----------------------------------------------------------------------------
(* The writer begins the next version. *)
Begin ==
    /\ writer = "idle" /\ writing < MaxVersions
    /\ writing' = writing + 1
    /\ IF Protocol = "inPlace"
       THEN /\ seenData' = [seenData EXCEPT ![seenName["file"]] = Empty]
            /\ writer' = "truncated"
            /\ UNCHANGED seenName
       ELSE /\ seenName' = [seenName EXCEPT !["temp"] = writing + 1]
            /\ writer' = "made"
            /\ UNCHANGED seenData
    /\ UNCHANGED <<diskName, diskData, acked, crashes>>

(* In place: the file is written in two parts. *)
WriteHalf ==
    /\ writer = "truncated"
    /\ seenData' = [seenData EXCEPT ![seenName["file"]] = Half(writing)]
    /\ writer' = "half"
    /\ UNCHANGED <<seenName, diskName, diskData, writing, acked, crashes>>

WriteRest ==
    /\ writer = "half"
    /\ seenData' = [seenData EXCEPT ![seenName["file"]] = Whole(writing)]
    /\ Done(writing)
    /\ UNCHANGED <<seenName, diskName, diskData>>

(* Renaming: the temporary file is written whole, then maybe fsynced. *)
WriteTemp ==
    /\ writer = "made"
    /\ seenData' = [seenData EXCEPT ![writing] = Whole(writing)]
    /\ writer' = IF Protocol = "rename" THEN "synced" ELSE "written"
    /\ UNCHANGED <<seenName, diskName, diskData, writing, acked, crashes>>

SyncTemp ==
    /\ writer = "written"
    /\ diskData' = [diskData EXCEPT ![writing] = seenData[writing]]
    /\ writer' = "synced"
    /\ UNCHANGED <<seenName, diskName, seenData, writing, acked, crashes>>

Rename ==
    /\ writer = "synced"
    /\ seenName' = [seenName EXCEPT !["file"] = seenName["temp"], !["temp"] = NoInode]
    /\ IF Protocol = "renameSyncedDir"
       THEN /\ writer' = "renamed" /\ UNCHANGED <<acked, writing, crashes>>
       ELSE Done(writing)
    /\ UNCHANGED <<diskName, seenData, diskData>>

SyncDir ==
    /\ writer = "renamed"
    /\ diskName' = seenName
    /\ Done(writing)
    /\ UNCHANGED <<seenName, seenData, diskData>>

-----------------------------------------------------------------------------
(* The kernel writes an inode's data, or the directory's names, on its own. *)
FlushData ==
    /\ \E i \in Inodes :
         /\ diskData[i] # seenData[i]
         /\ diskData' = [diskData EXCEPT ![i] = seenData[i]]
    /\ UNCHANGED <<seenName, diskName, seenData, writer, writing, acked, crashes>>

FlushNames ==
    /\ diskName # seenName
    /\ diskName' = seenName
    /\ UNCHANGED <<seenName, seenData, diskData, writer, writing, acked, crashes>>

(* The process dies: the write stops where it is; the page cache stays. *)
ProcessCrash ==
    /\ crashes < MaxCrashes /\ writer # "idle"
    /\ crashes' = crashes + 1
    /\ writer' = "idle"
    /\ UNCHANGED <<seenName, diskName, seenData, diskData, writing, acked>>

(* The host dies: what is on disk is all there is. *)
HostCrash ==
    /\ crashes < MaxCrashes
    /\ crashes' = crashes + 1
    /\ seenName' = diskName
    /\ seenData' = diskData
    /\ writer' = "idle"
    /\ UNCHANGED <<diskName, diskData, writing, acked>>

Next ==
    \/ Begin \/ WriteHalf \/ WriteRest \/ WriteTemp \/ SyncTemp \/ Rename \/ SyncDir
    \/ FlushData \/ FlushNames \/ ProcessCrash \/ HostCrash

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* What a reader finds at the file's name: what gusto reads on start. *)
Read == IF seenName["file"] = NoInode THEN Empty ELSE seenData[seenName["file"]]

(* The file is always a whole version: a reader never finds it empty or  *)
(* torn, so a task or a thread is never dropped as unreadable.           *)
AlwaysWhole == Read.kind = "whole"

(* No version the writer said was done is lost, by either crash. *)
NothingDoneIsLost == Read.kind = "whole" => Read.version >= acked
=============================================================================
