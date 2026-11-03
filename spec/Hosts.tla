------------------------------- MODULE Hosts --------------------------------
(***************************************************************************)
(* The hosts organizations' servers run on, as the scaler stops them and   *)
(* the controller places on them (internal/cell scaler.go, pool.go,        *)
(* internal/cloud controller.go).  An organization is provisioned: it is   *)
(* placed on a host, or stays on the host it was placed on, and its server *)
(* is started there.  The scaler retires a host it judges idle: it marks   *)
(* it, checks it again, and stops it.  A stopped host may be started       *)
(* again, and registers.  A host's registration is what the pool reads:    *)
(* while it is there, a silent host is taken to be away for a while, and   *)
(* one placed on is tried again; once it is not, the host is gone, and its *)
(* organizations are placed again.                                         *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "unregister": the scaler removes the registration of a host it      *)
(*    stops or removes;                                                    *)
(*  - "busy": an organization being placed or started keeps every host     *)
(*    from being judged idle, as one that waits for room does;            *)
(*  - "mark": a host being retired is not placed on or started on, from    *)
(*    before it is checked the last time.                                  *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, Hosts, Orgs, MaxStops

ASSUME Fixes \subseteq {"unregister", "busy", "mark"}

None == "none"

VARIABLES
    up,       \* each host is running
    reg,      \* each host's registration is there
    marked,   \* each host is being retired
    checked,  \* each marked host was found idle, and is about to be stopped
    servers,  \* the organizations whose server runs on each host
    org,      \* each organization: its state, and the host it is placed on
    wrong,    \* a host was stopped while an organization's server was being started on it
    stops

vars == <<up, reg, marked, checked, servers, org, wrong, stops>>

States == {"none", "provisioning", "starting", "ready", "failed"}

TypeOK ==
    /\ up \in [Hosts -> BOOLEAN] /\ reg \in [Hosts -> BOOLEAN]
    /\ marked \in [Hosts -> BOOLEAN] /\ checked \in [Hosts -> BOOLEAN]
    /\ servers \in [Hosts -> SUBSET Orgs]
    /\ org \in [Orgs -> [st : States, cell : Hosts \cup {None}]]
    /\ wrong \in BOOLEAN /\ stops \in 0..MaxStops

Init ==
    /\ up = [h \in Hosts |-> TRUE] /\ reg = [h \in Hosts |-> TRUE]
    /\ marked = [h \in Hosts |-> FALSE] /\ checked = [h \in Hosts |-> FALSE]
    /\ servers = [h \in Hosts |-> {}]
    /\ org = [o \in Orgs |-> [st |-> "none", cell |-> None]]
    /\ wrong = FALSE /\ stops = 0

(* Whether the controller may put a server on the host. *)
Open(h) == up[h] /\ ~("mark" \in Fixes /\ marked[h])

InFlight == \E o \in Orgs : org[o].st \in {"provisioning", "starting"}

-----------------------------------------------------------------------------
Provision(o) ==
    /\ org[o].st \in {"none", "failed"}
    /\ org' = [org EXCEPT ![o].st = "provisioning"]
    /\ UNCHANGED <<up, reg, marked, checked, servers, wrong, stops>>

(* placement: a host the organization is on is kept while it is registered: *)
(* Get gives it, or says it cannot be reached.  Otherwise Place gives the   *)
(* host with room.                                                          *)
Place(o) ==
    /\ org[o].st = "provisioning"
    /\ LET c == org[o].cell IN
       IF c # None /\ reg[c]
       THEN org' = [org EXCEPT ![o].st = IF Open(c) THEN "starting" ELSE "failed"]
       ELSE IF \E h \in Hosts : reg[h] /\ Open(h)
            THEN \E h \in Hosts : reg[h] /\ Open(h) /\ org' = [org EXCEPT ![o] = [st |-> "starting", cell |-> h]]
            ELSE org' = [org EXCEPT ![o] = [st |-> "failed", cell |-> None]]
    /\ UNCHANGED <<up, reg, marked, checked, servers, wrong, stops>>

Start(o) ==
    /\ org[o].st = "starting"
    /\ LET c == org[o].cell IN
       IF up[c]
       THEN /\ servers' = [servers EXCEPT ![c] = @ \cup {o}]
            /\ org' = [org EXCEPT ![o].st = "ready"]
       ELSE /\ org' = [org EXCEPT ![o].st = "failed"] /\ UNCHANGED servers
    /\ UNCHANGED <<up, reg, marked, checked, wrong, stops>>

(* A server ends on its own, and its host may then be idle. *)
Crash(o) ==
    /\ org[o].st = "ready" /\ o \in servers[org[o].cell]
    /\ servers' = [servers EXCEPT ![org[o].cell] = @ \ {o}]
    /\ UNCHANGED <<up, reg, marked, checked, org, wrong, stops>>

(* supervise finds a ready server gone. *)
Lost(o) ==
    /\ org[o].st = "ready" /\ o \notin servers[org[o].cell]
    /\ org' = [org EXCEPT ![o].st = "failed"]
    /\ UNCHANGED <<up, reg, marked, checked, servers, wrong, stops>>

-----------------------------------------------------------------------------
(* retire: a host idle for long enough is marked, *)
Mark(h) ==
    /\ up[h] /\ ~marked[h] /\ servers[h] = {} /\ stops < MaxStops
    /\ marked' = [marked EXCEPT ![h] = TRUE]
    /\ UNCHANGED <<up, reg, checked, servers, org, wrong, stops>>

(* checked again, *)
Check(h) ==
    /\ marked[h] /\ ~checked[h]
    /\ IF servers[h] = {} /\ ~("busy" \in Fixes /\ InFlight)
       THEN checked' = [checked EXCEPT ![h] = TRUE] /\ UNCHANGED marked
       ELSE marked' = [marked EXCEPT ![h] = FALSE] /\ UNCHANGED checked
    /\ UNCHANGED <<up, reg, servers, org, wrong, stops>>

(* and stopped. *)
Stop(h) ==
    /\ checked[h] /\ stops < MaxStops
    /\ wrong' = (wrong \/ \E o \in Orgs : org[o].st = "starting" /\ org[o].cell = h)
    /\ up' = [up EXCEPT ![h] = FALSE] /\ servers' = [servers EXCEPT ![h] = {}]
    /\ reg' = IF "unregister" \in Fixes THEN [reg EXCEPT ![h] = FALSE] ELSE reg
    /\ marked' = [marked EXCEPT ![h] = FALSE] /\ checked' = [checked EXCEPT ![h] = FALSE]
    /\ stops' = stops + 1
    /\ UNCHANGED org

(* bringUp: a stopped host is started again and registers. *)
Up(h) ==
    /\ ~up[h]
    /\ up' = [up EXCEPT ![h] = TRUE] /\ reg' = [reg EXCEPT ![h] = TRUE]
    /\ UNCHANGED <<marked, checked, servers, org, wrong, stops>>

Next ==
    \/ \E o \in Orgs : Provision(o) \/ Place(o) \/ Start(o) \/ Crash(o) \/ Lost(o)
    \/ \E h \in Hosts : Mark(h) \/ Check(h) \/ Stop(h) \/ Up(h)

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* No host is stopped under an organization whose server is being started on it. *)
NoStopUnderAStart == ~wrong

(* An organization on a host that is down is not kept there: the pool says the host is gone. *)
DownIsGone == \A o \in Orgs : org[o].cell # None /\ ~up[org[o].cell] => ~reg[org[o].cell]
=============================================================================
