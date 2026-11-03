------------------------------- MODULE Quota --------------------------------
(***************************************************************************)
(* One organization's quota file (internal/cloud billing.go), which the    *)
(* organization's server reads for its limits, and the database it is     *)
(* worked out from: the subscription, and whether an admin confirmed their *)
(* address.  Writers change the database: a Stripe event, the seat sync,   *)
(* a confirmed address.  After its change each calls ApplyQuota, which     *)
(* reads the database and writes the file.  Writing the file may fail,     *)
(* and the front door may crash; a start writes every quota again.  db     *)
(* counts the changes, and the file holds the count it was worked out      *)
(* from.                                                                   *)
(*                                                                         *)
(* A Stripe event whose quota was not written is answered with an error,   *)
(* and Stripe sends it again; the other writers log the error.  Role       *)
(* changes and removals are not writers: an organization with no admin    *)
(* who confirmed their address is on the trial, which has room for one     *)
(* person, or it has a subscription, which decides its quota.              *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "serial": ApplyQuota reads and writes under the organization's row,  *)
(*    which a second ApplyQuota and a writer's mark wait for;              *)
(*  - "dueMark": each writer marks the quota due in its transaction,       *)
(*    ApplyQuota clears the mark when it writes the file, and the leader's *)
(*    sweep writes every quota still marked.                               *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Fixes,
    Appliers,     \* the calls of ApplyQuota that may run at once
    MaxWrites,
    MaxCrashes,
    MaxFailures   \* how many file writes fail

ASSUME Fixes \subseteq {"serial", "dueMark"}

Serial == "serial" \in Fixes
Marks == "dueMark" \in Fixes

VARIABLES
    db,       \* how many changes the database holds
    file,     \* the change count the file was worked out from
    due,      \* with dueMark, the organization's row is marked
    call,     \* each ApplyQuota call: idle, due to run, or holding what it read
    kind,     \* whether each call was made for a Stripe event, which Stripe sends again
    read,     \* what each call read
    lock,     \* with serial, the call holding the organization's row, or 0
    up, crashes, failures

vars == <<db, file, due, call, kind, read, lock, up, crashes, failures>>

TypeOK ==
    /\ db \in 0..MaxWrites /\ file \in 0..MaxWrites /\ due \in BOOLEAN
    /\ call \in [Appliers -> {"idle", "due", "read"}]
    /\ kind \in [Appliers -> {"event", "other"}]
    /\ read \in [Appliers -> 0..MaxWrites]
    /\ lock \in Appliers \cup {0}
    /\ up \in BOOLEAN /\ crashes \in 0..MaxCrashes /\ failures \in 0..MaxFailures

Init ==
    /\ db = 0 /\ file = 0 /\ due = FALSE
    /\ call = [a \in Appliers |-> "idle"] /\ kind = [a \in Appliers |-> "other"]
    /\ read = [a \in Appliers |-> 0] /\ lock = 0
    /\ up = TRUE /\ crashes = 0 /\ failures = 0

-----------------------------------------------------------------------------
(* A writer commits its change and calls ApplyQuota after.  With dueMark  *)
(* it marks the row, which waits for an ApplyQuota that holds it.          *)
Write(k) ==
    /\ up /\ db < MaxWrites
    /\ ~(Marks /\ lock # 0)
    /\ db' = db + 1
    /\ due' = (due \/ Marks)
    /\ \E a \in Appliers :
         /\ call[a] = "idle"
         /\ call' = [call EXCEPT ![a] = "due"]
         /\ kind' = [kind EXCEPT ![a] = k]
    /\ UNCHANGED <<file, read, lock, up, crashes, failures>>

(* ApplyQuota reads what the quota is worked out from; with serial, under  *)
(* the organization's row.                                                 *)
Read(a) ==
    /\ up /\ call[a] = "due"
    /\ Serial => lock = 0
    /\ read' = [read EXCEPT ![a] = db]
    /\ call' = [call EXCEPT ![a] = "read"]
    /\ lock' = IF Serial THEN a ELSE lock
    /\ UNCHANGED <<db, file, due, kind, up, crashes, failures>>

(* It writes the file, and its transaction clears the mark and commits. *)
WriteFile(a) ==
    /\ up /\ call[a] = "read"
    /\ file' = read[a]
    /\ due' = (due /\ ~Marks)
    /\ call' = [call EXCEPT ![a] = "idle"]
    /\ lock' = IF lock = a THEN 0 ELSE lock
    /\ UNCHANGED <<db, kind, read, up, crashes, failures>>

(* The file is not written, and the transaction rolls back.  Stripe sends *)
(* an event again; any other caller logs the error.                        *)
FailFile(a) ==
    /\ up /\ call[a] = "read" /\ failures < MaxFailures
    /\ failures' = failures + 1
    /\ call' = [call EXCEPT ![a] = IF kind[a] = "event" THEN "due" ELSE "idle"]
    /\ lock' = IF lock = a THEN 0 ELSE lock
    /\ UNCHANGED <<db, file, due, kind, read, up, crashes>>

(* RunQuotas, on the leader: an organization still marked is written. *)
Sweep ==
    /\ up /\ Marks /\ due
    /\ \A b \in Appliers : call[b] = "idle"
    /\ \E a \in Appliers :
         /\ call' = [call EXCEPT ![a] = "due"]
         /\ kind' = [kind EXCEPT ![a] = "other"]
    /\ UNCHANGED <<db, file, due, read, lock, up, crashes, failures>>

(* The front door crashes: its calls and transactions are lost.  Stripe   *)
(* sends again an event it was not answered for.  ApplyQuotas writes      *)
(* every quota on the next start.                                          *)
Crash ==
    /\ up /\ crashes < MaxCrashes
    /\ up' = FALSE /\ crashes' = crashes + 1
    /\ call' = [a \in Appliers |-> IF kind[a] = "event" /\ call[a] # "idle" THEN "due" ELSE "idle"]
    /\ lock' = 0
    /\ UNCHANGED <<db, file, due, kind, read, failures>>

Restart ==
    /\ ~up /\ up' = TRUE
    /\ IF \E a \in Appliers : call[a] = "idle"
       THEN \E a \in Appliers :
              /\ call[a] = "idle"
              /\ call' = [call EXCEPT ![a] = "due"]
              /\ kind' = [kind EXCEPT ![a] = "other"]
       ELSE UNCHANGED <<call, kind>>
    /\ UNCHANGED <<db, file, due, read, lock, crashes, failures>>

Next ==
    \/ \E k \in {"event", "other"} : Write(k)
    \/ \E a \in Appliers : Read(a) \/ WriteFile(a) \/ FailFile(a)
    \/ Sweep \/ Crash \/ Restart

Fairness ==
    /\ \A a \in Appliers : SF_vars(Read(a)) /\ WF_vars(WriteFile(a))
    /\ WF_vars(Sweep) /\ WF_vars(Restart)

Spec == Init /\ [][Next]_vars /\ Fairness

-----------------------------------------------------------------------------
(* When no call is left to make and nothing is marked, the file says what *)
(* the database does.                                                      *)
Settled == up /\ ~due /\ \A a \in Appliers : call[a] = "idle"
QuotaFollows == Settled => file = db

(* However the changes go, the file comes to say what the database does. *)
Converges == <>[](file = db)
=============================================================================
