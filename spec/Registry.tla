------------------------------- MODULE Registry ------------------------------
(***************************************************************************)
(* One provider in an organization's model registry, and its model, as     *)
(* admins change them at once (internal/registry edit.go).  Adding the     *)
(* provider checks its name is free, keeps the key it was given, and then  *)
(* keeps the provider; when that is refused, it forgets the key again.     *)
(* Retiring the provider keeps it retired, then forgets its key.  Each     *)
(* admin adds with a key of their own.  A model is added to the provider,  *)
(* and priced.                                                             *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "lock": one change at a time, across the key it keeps or forgets;    *)
(*  - "models": retiring a provider retires its models, and a model whose  *)
(*    provider is retired is not changed.                                  *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, Admins, MaxChanges

ASSUME Fixes \subseteq {"lock", "models"}

Lock == "lock" \in Fixes
Models == "models" \in Fixes

VARIABLES
    provider,  \* the provider in the registry: none, live or retired
    given,     \* the key the provider was last added with, or none
    kept,      \* the key kept for it, or none
    model,     \* its model: none, live or retired
    changed,   \* a model of a retired provider was changed
    pc,        \* where each admin is
    holder,    \* the admin making a change, with lock
    changes

vars == <<provider, given, kept, model, changed, pc, holder, changes>>

None == "none"

TypeOK ==
    /\ provider \in {"none", "live", "retired"} /\ model \in {"none", "live", "retired"}
    /\ given \in {None} \cup Admins /\ kept \in {None} \cup Admins
    /\ changed \in BOOLEAN /\ holder \in {None} \cup Admins
    /\ pc \in [Admins -> {"idle", "adding", "kept", "refused", "retired"}]
    /\ changes \in 0..MaxChanges

Init ==
    /\ provider = "none" /\ given = None /\ kept = None /\ model = "none"
    /\ changed = FALSE /\ pc = [a \in Admins |-> "idle"] /\ holder = None /\ changes = 0

Free == holder = None \/ ~Lock

Begin(a, step) ==
    /\ pc[a] = "idle" /\ Free /\ changes < MaxChanges
    /\ pc' = [pc EXCEPT ![a] = step] /\ changes' = changes + 1
    /\ holder' = IF Lock THEN a ELSE holder

Done(a) == pc' = [pc EXCEPT ![a] = "idle"] /\ holder' = None

-----------------------------------------------------------------------------
(* AddProvider: the name is free. *)
StartAdd(a) ==
    /\ provider # "live"
    /\ Begin(a, "adding")
    /\ UNCHANGED <<provider, given, kept, model, changed>>

Keep(a) ==
    /\ pc[a] = "adding"
    /\ kept' = a /\ pc' = [pc EXCEPT ![a] = "kept"]
    /\ UNCHANGED <<provider, given, model, changed, holder, changes>>

(* The provider is kept, or refused when another admin added it meanwhile. *)
Write(a) ==
    /\ pc[a] = "kept"
    /\ IF provider # "live"
       THEN /\ provider' = "live" /\ given' = a
            /\ model' = IF model = "live" THEN "retired" ELSE model
            /\ Done(a)
       ELSE /\ pc' = [pc EXCEPT ![a] = "refused"]
            /\ UNCHANGED <<provider, given, model, holder>>
    /\ UNCHANGED <<kept, changed, changes>>

ForgetRefused(a) ==
    /\ pc[a] = "refused"
    /\ kept' = None /\ Done(a)
    /\ UNCHANGED <<provider, given, model, changed, changes>>

(* RetireProvider. *)
Retire(a) ==
    /\ provider = "live"
    /\ Begin(a, "retired")
    /\ provider' = "retired" /\ given' = None
    /\ model' = IF Models /\ model = "live" THEN "retired" ELSE model
    /\ UNCHANGED <<kept, changed>>

ForgetRetired(a) ==
    /\ pc[a] = "retired"
    /\ kept' = None /\ Done(a)
    /\ UNCHANGED <<provider, given, model, changed, changes>>

(* AddModel and SetPrice are each one change. *)
AddModel ==
    /\ Free /\ provider = "live" /\ model # "live"
    /\ model' = "live"
    /\ UNCHANGED <<provider, given, kept, changed, pc, holder, changes>>

SetPrice ==
    /\ Free /\ model = "live"
    /\ Models => provider = "live"
    /\ changed' = (changed \/ provider # "live")
    /\ UNCHANGED <<provider, given, kept, model, pc, holder, changes>>

Next ==
    \/ \E a \in Admins : StartAdd(a) \/ Keep(a) \/ Write(a) \/ ForgetRefused(a) \/ Retire(a) \/ ForgetRetired(a)
    \/ AddModel \/ SetPrice

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* Once every change has finished, the key kept is the one the provider    *)
(* was last added with, and none for a provider not in use.                *)
KeyIsTheLatest == (\A a \in Admins : pc[a] = "idle") => kept = given

(* A retired provider has no model in use, and none of its models is       *)
(* changed.                                                                *)
RetiredIsRetired == ~changed /\ (provider = "retired" => model # "live")
=============================================================================
