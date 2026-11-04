----------------------------- MODULE RoomMembers -----------------------------
(***************************************************************************)
(* Who and what is in one room (internal/instance membership.go,           *)
(* internal/web rooms_members.go, members_agents.go).  Ada runs the room.  *)
(* She adds Bob, and his agent may be added with him; that add can be     *)
(* refused for the agent.  Another admin may make Bob an admin at the same *)
(* time.  Bob can be taken out, and his agent goes with him.  The agent    *)
(* takes turns, and can be deleted.  Bob opens his own room with the agent *)
(* from two clicks at once.  The server can crash between two steps.       *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "join": the people and agents of an add are checked and added in one *)
(*    change, all of them or none;                                         *)
(*  - "together": a person taken out takes their agents in the same change;*)
(*  - "stop": deleting an agent stops its turns, as taking it out does;    *)
(*  - "once": finding a person's own room with an agent and making it are  *)
(*    one step.                                                            *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes

ASSUME Fixes \subseteq {"join", "together", "stop", "once"}

VARIABLES
    bob,       \* Bob's role in the room: none, member or admin
    agent,     \* Bob's agent is in the room
    turn,      \* a turn of the agent's is running
    adding,    \* where Ada's add of Bob is: idle, checked, added
    refused,   \* Ada's add was refused, and how much of it stayed
    demoted,   \* an add took Bob's admin away
    leaving,   \* Bob has been taken out and his agent not yet
    clicks,    \* where each click to open Bob's room is
    rooms,     \* how many rooms of Bob's own with the agent there are
    done       \* every step has been taken once

vars == <<bob, agent, turn, adding, refused, demoted, leaving, clicks, rooms, done>>

Clicks == {1, 2}

TypeOK ==
    /\ bob \in {"none", "member", "admin"} /\ agent \in BOOLEAN /\ turn \in BOOLEAN
    /\ adding \in {"idle", "checked", "added", "over"} /\ refused \in {"no", "clean", "partial"}
    /\ demoted \in BOOLEAN /\ leaving \in BOOLEAN
    /\ clicks \in [Clicks -> {"idle", "looked", "over"}] /\ rooms \in 0..2

Init ==
    /\ bob = "none" /\ agent = FALSE /\ turn = FALSE
    /\ adding = "idle" /\ refused = "no" /\ demoted = FALSE /\ leaving = FALSE
    /\ clicks = [c \in Clicks |-> "idle"] /\ rooms = 0 /\ done = {}

-----------------------------------------------------------------------------
(* Ada adds Bob.  With join the check and the add are one change.          *)
AddCheck ==
    /\ adding = "idle" /\ bob = "none"
    /\ IF "join" \in Fixes
       THEN /\ bob' = "member" /\ adding' = "over"
       ELSE /\ adding' = "checked" /\ UNCHANGED bob
    /\ UNCHANGED <<agent, turn, refused, demoted, leaving, clicks, rooms, done>>

AddSet ==
    /\ adding = "checked"
    /\ demoted' = (demoted \/ bob = "admin")
    /\ bob' = "member" /\ adding' = "added"
    /\ UNCHANGED <<agent, turn, refused, leaving, clicks, rooms, done>>

(* The add also names an agent that is refused. *)
AddRefused ==
    /\ adding \in {"idle", "added"} /\ refused = "no"
    /\ refused' = IF adding = "added" THEN "partial" ELSE "clean"
    /\ adding' = "over"
    /\ UNCHANGED <<bob, agent, turn, demoted, leaving, clicks, rooms, done>>

AddAgent ==
    /\ adding = "added" \/ (adding = "over" /\ refused = "no")
    /\ bob # "none" /\ ~agent /\ ~leaving /\ "agent" \notin done
    /\ agent' = TRUE /\ done' = done \cup {"agent"}
    /\ UNCHANGED <<bob, turn, adding, refused, demoted, leaving, clicks, rooms>>

MakeAdmin ==
    /\ bob = "member"
    /\ bob' = "admin"
    /\ UNCHANGED <<agent, turn, adding, refused, demoted, leaving, clicks, rooms, done>>

-----------------------------------------------------------------------------
Ask ==
    /\ agent /\ ~turn /\ "turn" \notin done
    /\ turn' = TRUE /\ done' = done \cup {"turn"}
    /\ UNCHANGED <<bob, agent, adding, refused, demoted, leaving, clicks, rooms>>

Finish ==
    /\ turn
    /\ turn' = FALSE
    /\ UNCHANGED <<bob, agent, adding, refused, demoted, leaving, clicks, rooms, done>>

(* Taking Bob out stops his agent's turns with it. *)
TakeOut ==
    /\ bob # "none" /\ ~leaving
    /\ bob' = "none"
    /\ IF "together" \in Fixes
       THEN agent' = FALSE /\ turn' = FALSE /\ UNCHANGED leaving
       ELSE leaving' = TRUE /\ UNCHANGED <<agent, turn>>
    /\ UNCHANGED <<adding, refused, demoted, clicks, rooms, done>>

TakeOutAgents ==
    /\ leaving
    /\ agent' = FALSE /\ turn' = FALSE /\ leaving' = FALSE
    /\ UNCHANGED <<bob, adding, refused, demoted, clicks, rooms, done>>

Crash ==
    /\ leaving /\ "crash" \notin done
    /\ leaving' = FALSE /\ done' = done \cup {"crash"}
    /\ UNCHANGED <<bob, agent, turn, adding, refused, demoted, clicks, rooms>>

DeleteAgent ==
    /\ agent
    /\ agent' = FALSE
    /\ IF "stop" \in Fixes THEN turn' = FALSE ELSE UNCHANGED turn
    /\ UNCHANGED <<bob, adding, refused, demoted, leaving, clicks, rooms, done>>

-----------------------------------------------------------------------------
(* A click finds Bob's own room with the agent, or makes one. *)
Look(c) ==
    /\ clicks[c] = "idle"
    /\ IF "once" \in Fixes
       THEN /\ rooms' = IF rooms = 0 THEN 1 ELSE rooms
            /\ clicks' = [clicks EXCEPT ![c] = "over"]
       ELSE /\ clicks' = [clicks EXCEPT ![c] = IF rooms = 0 THEN "looked" ELSE "over"]
            /\ UNCHANGED rooms
    /\ UNCHANGED <<bob, agent, turn, adding, refused, demoted, leaving, done>>

Make(c) ==
    /\ clicks[c] = "looked"
    /\ rooms' = rooms + 1 /\ clicks' = [clicks EXCEPT ![c] = "over"]
    /\ UNCHANGED <<bob, agent, turn, adding, refused, demoted, leaving, done>>

Next ==
    \/ AddCheck \/ AddSet \/ AddRefused \/ AddAgent \/ MakeAdmin
    \/ Ask \/ Finish \/ TakeOut \/ TakeOutAgents \/ Crash \/ DeleteAgent
    \/ \E c \in Clicks : Look(c) \/ Make(c)

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* No add takes an admin's role away, and one that is refused adds nobody. *)
AddsOnlyAdd == ~demoted /\ refused # "partial"

(* An agent whose owner is not in the room is not in it either, once      *)
(* nothing is being changed.                                               *)
AgentsFollowTheirOwner == ~leaving => (bob = "none" => ~agent)

(* No turn runs for an agent that is not in the room. *)
NoTurnWithoutItsAgent == turn => agent

(* A person has one room of their own with an agent. *)
OneOwnRoom == rooms <= 1
=============================================================================
