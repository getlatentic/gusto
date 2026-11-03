----------------------------- MODULE RoomGuard -----------------------------
(***************************************************************************)
(* One room's folder in the room-work hub (internal/roomwork): turns join *)
(* the room, merge what they changed under the room's guard, and leave;   *)
(* a person's upload or edit holds the guard while it writes (Hub.Hold).  *)
(* The hub keeps a state for the room, with its guard, while turns work   *)
(* in it, and drops it when the last leaves.                               *)
(*                                                                         *)
(* Fixes is which fixes the code has: "holdKeepsState" is that a hold     *)
(* makes the room's state when there is none and counts itself in it, and *)
(* the state is dropped only when no turn works in it and nothing holds   *)
(* it, so a merge and a held change always meet at one guard.             *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets

CONSTANTS
    Fixes,    \* a subset of {"holdKeepsState"}
    Turns,    \* the turns
    MaxIds    \* how many times the room's state may be made

ASSUME Fixes \subseteq {"holdKeepsState"}

Ids == 1..MaxIds
Keeps == "holdKeepsState" \in Fixes

VARIABLES
    current,   \* the room's state in Hub.rooms, or 0 for none
    made,      \* how many states have been made
    live,      \* each state's working turns
    holds,     \* with the fix, each state's holds
    guard,     \* each state's guard: free, or who has it
    turn,      \* where each turn is
    of,        \* the state each turn joined
    person,    \* where the person's change is
    heldIn     \* the state the person's hold took, or 0

vars == <<current, made, live, holds, guard, turn, of, person, heldIn>>

TypeOK ==
    /\ current \in 0..MaxIds /\ made \in 0..MaxIds
    /\ live \in [Ids -> SUBSET Turns] /\ holds \in [Ids -> 0..1]
    /\ guard \in [Ids -> Turns \cup {"free", "person"}]
    /\ turn \in [Turns -> {"out", "in", "merging", "done"}]
    /\ of \in [Turns -> 0..MaxIds]
    /\ person \in {"idle", "writing", "done"} /\ heldIn \in 0..MaxIds

Init ==
    /\ current = 0 /\ made = 0
    /\ live = [i \in Ids |-> {}] /\ holds = [i \in Ids |-> 0]
    /\ guard = [i \in Ids |-> "free"]
    /\ turn = [t \in Turns |-> "out"] /\ of = [t \in Turns |-> 0]
    /\ person = "idle" /\ heldIn = 0

-----------------------------------------------------------------------------
(* Hub.join: a turn joins the room's state. *)
Join(t) ==
    /\ turn[t] = "out"
    /\ LET joined(i) == /\ live' = [live EXCEPT ![i] = @ \cup {t}]
                        /\ of' = [of EXCEPT ![t] = i]
       IN IF current # 0
          THEN /\ joined(current) /\ UNCHANGED <<current, made>>
          ELSE /\ made < MaxIds /\ made' = made + 1 /\ current' = made + 1 /\ joined(made + 1)
    /\ turn' = [turn EXCEPT ![t] = "in"]
    /\ UNCHANGED <<holds, guard, person, heldIn>>

(* Turn.Merge takes the guard of the state the turn joined. *)
Merge(t) ==
    /\ turn[t] = "in" /\ guard[of[t]] = "free"
    /\ guard' = [guard EXCEPT ![of[t]] = t]
    /\ turn' = [turn EXCEPT ![t] = "merging"]
    /\ UNCHANGED <<current, made, live, holds, of, person, heldIn>>

Merged(t) ==
    /\ turn[t] = "merging"
    /\ guard' = [guard EXCEPT ![of[t]] = "free"]
    /\ turn' = [turn EXCEPT ![t] = "in"]
    /\ UNCHANGED <<current, made, live, holds, of, person, heldIn>>

(* Hub.leave: the state is dropped when no turn works in it, and with the *)
(* fix nothing holds it.                                                   *)
Leave(t) ==
    /\ turn[t] = "in"
    /\ LET i == of[t]
           rest == live[i] \ {t}
       IN /\ live' = [live EXCEPT ![i] = rest]
          /\ current' = IF current = i /\ rest = {} /\ (~Keeps \/ holds[i] = 0) THEN 0 ELSE current
    /\ turn' = [turn EXCEPT ![t] = "done"]
    /\ UNCHANGED <<made, holds, guard, of, person, heldIn>>

-----------------------------------------------------------------------------
(* Hub.Hold.  Without the fix, a room with no state has no merge to wait  *)
(* for, and the change goes ahead holding nothing.  With it, the state is *)
(* made, the hold counted in it, and its guard taken.                      *)
Hold ==
    /\ person = "idle"
    /\ IF Keeps
       THEN IF current # 0
            THEN /\ guard[current] = "free"
                 /\ guard' = [guard EXCEPT ![current] = "person"]
                 /\ holds' = [holds EXCEPT ![current] = 1]
                 /\ heldIn' = current
                 /\ UNCHANGED <<current, made>>
            ELSE /\ made < MaxIds
                 /\ made' = made + 1 /\ current' = made + 1
                 /\ guard' = [guard EXCEPT ![made + 1] = "person"]
                 /\ holds' = [holds EXCEPT ![made + 1] = 1]
                 /\ heldIn' = made + 1
       ELSE IF current # 0
            THEN /\ guard[current] = "free"
                 /\ guard' = [guard EXCEPT ![current] = "person"]
                 /\ heldIn' = current
                 /\ UNCHANGED <<current, made, holds>>
            ELSE UNCHANGED <<current, made, holds, guard, heldIn>>
    /\ person' = "writing"
    /\ UNCHANGED <<live, turn, of>>

(* done: the guard is let go; with the fix the hold is counted out and an *)
(* empty state dropped.                                                    *)
Done ==
    /\ person = "writing"
    /\ guard' = IF heldIn # 0 THEN [guard EXCEPT ![heldIn] = "free"] ELSE guard
    /\ holds' = IF heldIn # 0 THEN [holds EXCEPT ![heldIn] = 0] ELSE holds
    /\ current' = IF Keeps /\ heldIn # 0 /\ current = heldIn /\ live[heldIn] = {} THEN 0 ELSE current
    /\ person' = "done"
    /\ UNCHANGED <<made, live, turn, of, heldIn>>

Next ==
    \/ \E t \in Turns : Join(t) \/ Merge(t) \/ Merged(t) \/ Leave(t)
    \/ Hold \/ Done

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* A merge never changes the folder while a person's change writes to it. *)
Exclusive == ~(person = "writing" /\ \E t \in Turns : turn[t] = "merging")
=============================================================================
