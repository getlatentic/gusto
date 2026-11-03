---------------------------- MODULE FrontDoors ----------------------------
(***************************************************************************)
(* Several front doors on one database (docs/CLOUD-DATABASE.md), each     *)
(* running a Controller (internal/cloud/controller.go) over the same       *)
(* organization, placing it on cells and supervising its server.  Each    *)
(* action names the Go function it stands for.                             *)
(*                                                                         *)
(* Atomicity assumed:                                                      *)
(*  - a statement on the database is one step;                             *)
(*  - provision, under one front door's own lock for the slug, is several *)
(*    steps: read the runtime, place, start, mark ready.  Another front   *)
(*    door's lock does not hold it back;                                   *)
(*  - starting a server on a cell for a slug that has one there is a      *)
(*    no-op (Cell.Start).                                                  *)
(*                                                                         *)
(* Fixes is which fixes the code has.  "leader" is that one front door at *)
(* a time leads: it holds a lease row with an epoch, only it runs the     *)
(* controller, and a runtime write carries its epoch and is refused when  *)
(* the lease has moved on.  A leader may pause past its lease and act on  *)
(* when it wakes, until it notices.                                        *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets

CONSTANTS
    Fixes,      \* a subset of {"leader"}
    Doors,      \* the front doors
    Cells,      \* the cells
    NoCell,     \* a value that is no cell
    MaxEpoch    \* how many times the lease may change hands

ASSUME NoCell \notin Cells

VARIABLES
    rowCell,    \* runtime.cell in the database
    rowState,   \* runtime.state in the database
    servers,    \* the cells running the organization's server
    pc,         \* where each front door's provision is
    seen,       \* the runtime each front door's provision read
    chosen,     \* the cell each front door's provision placed it on
    mem,        \* each front door's Controller.states entry for the slug
    lease,      \* the lease row: who holds it, and its epoch
    leads       \* the epoch each front door believes it leads with, or 0

vars == <<rowCell, rowState, servers, pc, seen, chosen, mem, lease, leads>>

Leading == "leader" \in Fixes

(* A front door may act as the controller: any of them without the fix,  *)
(* only one that believes it leads with it.                               *)
Acts(d) == ~Leading \/ leads[d] > 0

(* A runtime write from d is taken: with the fix, only at the lease's     *)
(* current epoch.                                                          *)
Taken(d) == ~Leading \/ leads[d] = lease.epoch

States == {"", "Provisioning", "Starting", "Ready", "Failed"}

TypeOK ==
    /\ rowCell \in Cells \cup {NoCell}
    /\ rowState \in States
    /\ servers \subseteq Cells
    /\ pc \in [Doors -> {"idle", "read", "placed", "started"}]
    /\ seen \in [Doors -> Cells \cup {NoCell}]
    /\ chosen \in [Doors -> Cells \cup {NoCell}]
    /\ mem \in [Doors -> States]
    /\ lease \in [holder: Doors \cup {NoCell}, epoch: 0..MaxEpoch]
    /\ leads \in [Doors -> 0..MaxEpoch]

Init ==
    /\ rowCell = NoCell /\ rowState = ""
    /\ servers = {}
    /\ pc = [d \in Doors |-> "idle"]
    /\ seen = [d \in Doors |-> NoCell]
    /\ chosen = [d \in Doors |-> NoCell]
    /\ mem = [d \in Doors |-> ""]
    /\ lease = [holder |-> NoCell, epoch |-> 0]
    /\ leads = [d \in Doors |-> 0]

-----------------------------------------------------------------------------
(* Run, when a front door starts: an organization with a runtime is set  *)
(* Starting, in memory and in the database, and then provisioned.         *)
RunStarts(d) ==
    /\ Acts(d) /\ pc[d] = "idle" /\ mem[d] = ""
    /\ mem' = [mem EXCEPT ![d] = IF rowState # "" THEN "Starting" ELSE ""]
    /\ rowState' = IF rowState # "" /\ Taken(d) THEN "Starting" ELSE rowState
    /\ UNCHANGED <<rowCell, servers, pc, seen, chosen, lease, leads>>

(* provision: Store.Runtime, then set Provisioning. *)
(* With the fix, a refused write ends the provision: the front door no   *)
(* longer leads.                                                           *)
Refuse(d) ==
    /\ pc' = [pc EXCEPT ![d] = "idle"]
    /\ leads' = [leads EXCEPT ![d] = 0]
    /\ UNCHANGED <<rowCell, rowState, servers, seen, chosen, mem, lease>>

Read(d) ==
    /\ Acts(d) /\ pc[d] = "idle"
    /\ IF Taken(d)
       THEN /\ seen' = [seen EXCEPT ![d] = rowCell]
            /\ rowState' = "Provisioning"
            /\ mem' = [mem EXCEPT ![d] = "Provisioning"]
            /\ pc' = [pc EXCEPT ![d] = "read"]
            /\ UNCHANGED <<rowCell, servers, chosen, lease, leads>>
       ELSE Refuse(d)

(* placement: an organization placed stays; one not placed goes on the   *)
(* cell with most room, written down before anything starts.              *)
Place(d) ==
    /\ pc[d] = "read"
    /\ IF Taken(d)
       THEN /\ IF seen[d] # NoCell
               THEN /\ chosen' = [chosen EXCEPT ![d] = seen[d]]
                    /\ UNCHANGED rowCell
               ELSE \E c \in Cells :
                      /\ chosen' = [chosen EXCEPT ![d] = c]
                      /\ rowCell' = c
            /\ rowState' = "Starting"
            /\ mem' = [mem EXCEPT ![d] = "Starting"]
            /\ pc' = [pc EXCEPT ![d] = "placed"]
            /\ UNCHANGED <<servers, seen, lease, leads>>
       ELSE Refuse(d)

(* cell.Start *)
Start(d) ==
    /\ pc[d] = "placed"
    /\ servers' = servers \cup {chosen[d]}
    /\ pc' = [pc EXCEPT ![d] = "started"]
    /\ UNCHANGED <<rowCell, rowState, seen, chosen, mem, lease, leads>>

(* The server answers: set Ready, as this front door placed it. *)
Ready(d) ==
    /\ pc[d] = "started"
    /\ IF Taken(d)
       THEN /\ rowState' = "Ready"
            /\ rowCell' = chosen[d]
            /\ mem' = [mem EXCEPT ![d] = "Ready"]
            /\ pc' = [pc EXCEPT ![d] = "idle"]
            /\ UNCHANGED <<servers, seen, chosen, lease, leads>>
       ELSE Refuse(d)

(* A server stops. *)
Dies(c) ==
    /\ c \in servers
    /\ servers' = servers \ {c}
    /\ UNCHANGED <<rowCell, rowState, pc, seen, chosen, mem, lease, leads>>

(* supervise: a server this front door holds Ready, and that is not alive *)
(* on the cell the database names, is provisioned again.                  *)
Supervise(d) ==
    /\ pc[d] = "idle" /\ mem[d] = "Ready" /\ rowCell \notin servers
    /\ Read(d)

-----------------------------------------------------------------------------
(* The lease.  A front door takes it when nobody holds it, at the next    *)
(* epoch, and starts its controller afresh.  The holder's lease may lapse *)
(* while it is paused; it notices when it next renews, and stops.          *)
Acquire(d) ==
    /\ Leading /\ lease.holder = NoCell /\ lease.epoch < MaxEpoch /\ leads[d] = 0
    /\ pc[d] = "idle"
    /\ lease' = [holder |-> d, epoch |-> lease.epoch + 1]
    /\ leads' = [leads EXCEPT ![d] = lease.epoch + 1]
    /\ mem' = [mem EXCEPT ![d] = ""]
    /\ UNCHANGED <<rowCell, rowState, servers, pc, seen, chosen>>

Lapse ==
    /\ Leading /\ lease.holder # NoCell
    /\ lease' = [lease EXCEPT !.holder = NoCell]
    /\ UNCHANGED <<rowCell, rowState, servers, pc, seen, chosen, mem, leads>>

Notice(d) ==
    /\ Leading /\ leads[d] > 0 /\ lease.holder # d
    /\ leads' = [leads EXCEPT ![d] = 0]
    /\ pc' = [pc EXCEPT ![d] = "idle"]
    /\ UNCHANGED <<rowCell, rowState, servers, seen, chosen, mem, lease>>

Next ==
    \E d \in Doors :
        \/ RunStarts(d) \/ (mem[d] = "" /\ Read(d)) \/ Place(d) \/ Start(d) \/ Ready(d)
        \/ Supervise(d)
        \/ Acquire(d) \/ Notice(d)
    \/ \E c \in Cells : Dies(c)
    \/ Lapse

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* One server for the organization, on the cell the database names. *)
OneServer == Cardinality(servers) <= 1

(* The database never says Ready for a cell that runs no server, unless  *)
(* the server has stopped since and the supervisor has yet to see it.     *)
ReadyIsWhereItRuns == rowState = "Ready" => rowCell \in servers \/ servers = {}
=============================================================================
