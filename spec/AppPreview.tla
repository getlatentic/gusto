---------------------------- MODULE AppPreview ----------------------------
(***************************************************************************)
(* An agent's shared app previews (internal/apps, internal/web/sharing.go, *)
(* internal/backend/vm): publishing one under the agent's limit, starting *)
(* its process in the owner's computer, the proxy finding it not          *)
(* answering, removing it, the process dying by itself, and the server    *)
(* restarting.  Each action names the Go function it stands for.          *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "orphanStopped": removing an app that is starting marks it          *)
(*    Removing, and its start stops what it started and removes it;       *)
(*    removing any other app stops it whatever its state;                 *)
(*  - "errorChecks": a proxy error has the app asked again, and only an   *)
(*    app that does not answer is stopped, and its process with it;       *)
(*  - "reconcile": an app left starting or removing by a restart is       *)
(*    stopped, and marked failed or removed, when the server starts;      *)
(*  - "limitInStore": the store counts the agent's apps and adds one in   *)
(*    one step.                                                            *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets

CONSTANTS
    Fixes,       \* a subset of the four above
    Apps,        \* the apps that may be published
    Limit,       \* how many an agent may have starting or live
    MaxCrashes

ASSUME Fixes \subseteq {"orphanStopped", "errorChecks", "reconcile", "limitInStore"}

Running == {"Starting", "Live"}

VARIABLES
    rec,      \* each app's record: none, Starting, Live, Stopped, Failed, Removing, or gone once removed
    proc,     \* each app's process runs in the computer, with its address exposed
    starter,  \* where each app's start is
    pub,      \* where each publish is
    checking, \* with errorChecks, an app is being asked again after a proxy error
    up,
    crashes

vars == <<rec, proc, starter, pub, checking, up, crashes>>

TypeOK ==
    /\ rec \in [Apps -> {"none", "Starting", "Live", "Stopped", "Failed", "Removing", "gone"}]
    /\ proc \in [Apps -> BOOLEAN]
    /\ starter \in [Apps -> {"idle", "launching", "launched"}]
    /\ pub \in [Apps -> {"idle", "checked"}]
    /\ checking \in [Apps -> BOOLEAN]
    /\ up \in BOOLEAN /\ crashes \in 0..MaxCrashes

Init ==
    /\ rec = [a \in Apps |-> "none"] /\ proc = [a \in Apps |-> FALSE]
    /\ starter = [a \in Apps |-> "idle"] /\ pub = [a \in Apps |-> "idle"]
    /\ checking = [a \in Apps |-> FALSE] /\ up = TRUE /\ crashes = 0

Counted == {a \in Apps : rec[a] \in Running}

-----------------------------------------------------------------------------
(* publishApp: sharesTooMany reads the count, then apps.New adds the      *)
(* record, and the start is begun.  With limitInStore the two are one.    *)
Check(a) ==
    /\ up /\ rec[a] = "none" /\ pub[a] = "idle" /\ "limitInStore" \notin Fixes
    /\ Cardinality(Counted) < Limit
    /\ pub' = [pub EXCEPT ![a] = "checked"]
    /\ UNCHANGED <<rec, proc, starter, checking, up, crashes>>

Add(a) ==
    /\ up /\ rec[a] = "none"
    /\ IF "limitInStore" \in Fixes THEN Cardinality(Counted) < Limit ELSE pub[a] = "checked"
    /\ rec' = [rec EXCEPT ![a] = "Starting"]
    /\ starter' = [starter EXCEPT ![a] = "launching"]
    /\ pub' = [pub EXCEPT ![a] = "idle"]
    /\ UNCHANGED <<proc, checking, up, crashes>>

(* sharedApps.Start: the process starts and its port is exposed. *)
Launch(a) ==
    /\ up /\ starter[a] = "launching"
    /\ proc' = [proc EXCEPT ![a] = TRUE]
    /\ starter' = [starter EXCEPT ![a] = "launched"]
    /\ UNCHANGED <<rec, pub, checking, up, crashes>>

(* The app answers, and startApp records it live.  Its record may have   *)
(* been removed meanwhile: the change then fails, and with orphanStopped *)
(* what was started is stopped.                                           *)
Answered(a) ==
    /\ up /\ starter[a] = "launched"
    /\ CASE rec[a] = "Starting" -> rec' = [rec EXCEPT ![a] = "Live"] /\ UNCHANGED proc
          [] rec[a] = "Removing" -> rec' = [rec EXCEPT ![a] = "gone"] /\ proc' = [proc EXCEPT ![a] = FALSE]
          [] OTHER -> UNCHANGED <<rec, proc>>
    /\ starter' = [starter EXCEPT ![a] = "idle"]
    /\ UNCHANGED <<pub, checking, up, crashes>>

(* The app does not answer in time: what was started is stopped, and the *)
(* record says why.                                                        *)
StartFails(a) ==
    /\ up /\ starter[a] \in {"launching", "launched"}
    /\ proc' = [proc EXCEPT ![a] = FALSE]
    /\ rec' = [rec EXCEPT ![a] = CASE @ = "Starting" -> "Failed" [] @ = "Removing" -> "gone" [] OTHER -> @]
    /\ starter' = [starter EXCEPT ![a] = "idle"]
    /\ UNCHANGED <<pub, checking, up, crashes>>

(* stopApp: the runner is asked to stop a live or starting app, which it *)
(* can only when the record names the sandbox, which a starting one's     *)
(* does not; then the record goes.  With orphanStopped the app is stopped *)
(* in the owner's computer whatever its state.                            *)
Remove(a) ==
    /\ up /\ rec[a] \in {"Starting", "Live", "Stopped", "Failed"}
    /\ proc' = [proc EXCEPT ![a] = IF "orphanStopped" \in Fixes \/ rec[a] = "Live" THEN FALSE ELSE @]
    /\ rec' = [rec EXCEPT ![a] = IF "orphanStopped" \in Fixes /\ @ = "Starting" THEN "Removing" ELSE "gone"]
    /\ UNCHANGED <<starter, pub, checking, up, crashes>>

(* The proxy's ErrorHandler: a request fails without a timeout, as one   *)
(* reset connection does.  Without errorChecks the app is marked stopped  *)
(* then and there, its process left as it is.                             *)
ProxyError(a) ==
    /\ up /\ rec[a] = "Live" /\ ~checking[a]
    /\ IF "errorChecks" \in Fixes
       THEN /\ checking' = [checking EXCEPT ![a] = TRUE] /\ UNCHANGED rec
       ELSE /\ rec' = [rec EXCEPT ![a] = "Stopped"] /\ UNCHANGED checking
    /\ UNCHANGED <<proc, starter, pub, up, crashes>>

(* checkApp: asked again, an app that answers stays live; one that does   *)
(* not is stopped and marked so.                                          *)
Recheck(a) ==
    /\ up /\ checking[a]
    /\ IF proc[a] /\ rec[a] = "Live"
       THEN UNCHANGED <<rec, proc>>
       ELSE /\ proc' = [proc EXCEPT ![a] = FALSE]
            /\ rec' = [rec EXCEPT ![a] = IF @ = "Live" THEN "Stopped" ELSE @]
    /\ checking' = [checking EXCEPT ![a] = FALSE]
    /\ UNCHANGED <<starter, pub, up, crashes>>

(* The app's process dies by itself. *)
Dies(a) ==
    /\ proc[a] /\ starter[a] = "idle"
    /\ proc' = [proc EXCEPT ![a] = FALSE]
    /\ UNCHANGED <<rec, starter, pub, checking, up, crashes>>

Crash ==
    /\ up /\ crashes < MaxCrashes
    /\ up' = FALSE /\ crashes' = crashes + 1
    /\ starter' = [a \in Apps |-> "idle"] /\ pub' = [a \in Apps |-> "idle"]
    /\ checking' = [a \in Apps |-> FALSE]
    /\ UNCHANGED <<rec, proc>>

(* The server starts.  With reconcile an app left starting is stopped and *)
(* marked failed.                                                          *)
Restart ==
    /\ ~up /\ up' = TRUE
    /\ IF "reconcile" \in Fixes
       THEN /\ rec' = [a \in Apps |-> CASE rec[a] = "Starting" -> "Failed" [] rec[a] = "Removing" -> "gone" [] OTHER -> rec[a]]
            /\ proc' = [a \in Apps |-> IF rec[a] \in {"Starting", "Removing"} THEN FALSE ELSE proc[a]]
       ELSE UNCHANGED <<rec, proc>>
    /\ UNCHANGED <<starter, pub, checking, crashes>>

Next ==
    \/ \E a \in Apps :
         Check(a) \/ Add(a) \/ Launch(a) \/ Answered(a) \/ StartFails(a)
         \/ Remove(a) \/ ProxyError(a) \/ Recheck(a) \/ Dies(a)
    \/ Crash \/ Restart

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* A process runs only for an app whose record shows it starting or live, *)
(* or while its start is under way: nothing runs, exposed, that the page  *)
(* no longer shows or calls stopped.                                      *)
NothingRunsUnseen == \A a \in Apps : proc[a] => rec[a] \in Running \cup {"Removing"} \/ starter[a] # "idle"

(* An app shows as starting only while its start is under way. *)
NoStuckStart == \A a \in Apps : rec[a] \in {"Starting", "Removing"} /\ up => starter[a] # "idle"

(* An agent has no more apps starting or live than its limit. *)
WithinLimit == Cardinality(Counted) <= Limit
=============================================================================
