------------------------------- MODULE AgentApps -----------------------------
(***************************************************************************)
(* What one person chose for an agent and one app of theirs (internal/crew *)
(* crew.go, internal/web connectors.go).  The person works with the agent  *)
(* or not, as its owner decides, and is connected to the app or not.  They *)
(* choose to let the agent use the app; a turn they ask of the agent uses  *)
(* it by that choice.  Each connection is numbered.                        *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "member": only a person who works with the agent chooses for it, and *)
(*    one taken off it loses what they chose;                              *)
(*  - "forget": ending the connection forgets what was chosen under it.    *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, MaxConnections

ASSUME Fixes \subseteq {"member", "forget"}

Member == "member" \in Fixes

VARIABLES
    works,      \* the person works with the agent
    connected,  \* the person is connected to the app
    connection, \* the number of the latest connection
    chosen,     \* the person let the agent use the app
    chosenIn    \* the connection the choice was made in

vars == <<works, connected, connection, chosen, chosenIn>>

TypeOK ==
    /\ works \in BOOLEAN /\ connected \in BOOLEAN /\ chosen \in BOOLEAN
    /\ connection \in 1..MaxConnections /\ chosenIn \in 0..MaxConnections

Init == works = TRUE /\ connected = TRUE /\ connection = 1 /\ chosen = FALSE /\ chosenIn = 0

Choose ==
    /\ connected /\ ~chosen
    /\ Member => works
    /\ chosen' = TRUE /\ chosenIn' = connection
    /\ UNCHANGED <<works, connected, connection>>

TakeOff ==
    /\ works
    /\ works' = FALSE
    /\ IF Member THEN chosen' = FALSE ELSE UNCHANGED chosen
    /\ UNCHANGED <<connected, connection, chosenIn>>

LetBack ==
    /\ ~works
    /\ works' = TRUE
    /\ UNCHANGED <<connected, connection, chosen, chosenIn>>

(* The choices are forgotten, then the tokens. *)
Disconnect ==
    /\ connected
    /\ connected' = FALSE
    /\ IF "forget" \in Fixes THEN chosen' = FALSE ELSE UNCHANGED chosen
    /\ UNCHANGED <<works, connection, chosenIn>>

Connect ==
    /\ ~connected /\ connection < MaxConnections
    /\ connected' = TRUE /\ connection' = connection + 1
    /\ UNCHANGED <<works, chosen, chosenIn>>

Next == Choose \/ TakeOff \/ LetBack \/ Disconnect \/ Connect

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* A choice is held only for a person who works with the agent. *)
ChoiceIsAMembers == chosen => works

(* A choice that a turn would use was made in the connection it uses. *)
ChoiceIsOfThisConnection == (chosen /\ connected) => chosenIn = connection
=============================================================================
