------------------------------ MODULE RoomSocket -----------------------------
(***************************************************************************)
(* One person's socket subscribing to a room while they are taken out of  *)
(* it (internal/roomlive connect.go).  The subscription checks the person *)
(* is in the room, and the socket is subscribed.  Taking the person out    *)
(* asks every socket watching the room to be checked again (Revalidate),  *)
(* which unsubscribes those whose person left; a socket not yet           *)
(* subscribed is not unsubscribed by it.                                   *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "watch": the socket is a watcher before the check, and the person is *)
(*    checked again once it is subscribed.                                 *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes

ASSUME Fixes \subseteq {"watch"}

Watch == "watch" \in Fixes

VARIABLES
    member,    \* the person is in the room
    pc,        \* the subscription: idle, watched, let, checking, subscribed, refused
    watching,  \* Revalidate finds the socket
    owed       \* a Revalidate is owed for the person taken out

vars == <<member, pc, watching, owed>>

TypeOK ==
    /\ member \in BOOLEAN /\ watching \in BOOLEAN /\ owed \in BOOLEAN
    /\ pc \in {"idle", "watched", "let", "checking", "subscribed", "refused"}

Init == member = TRUE /\ pc = "idle" /\ watching = FALSE /\ owed = FALSE

StartWatching ==
    /\ Watch /\ pc = "idle"
    /\ watching' = TRUE /\ pc' = "watched"
    /\ UNCHANGED <<member, owed>>

Check ==
    /\ pc = IF Watch THEN "watched" ELSE "idle"
    /\ IF member THEN pc' = "let" /\ UNCHANGED watching ELSE pc' = "refused" /\ watching' = FALSE
    /\ UNCHANGED <<member, owed>>

(* done: the socket is subscribed; without watch it becomes a watcher then. *)
Subscribe ==
    /\ pc = "let"
    /\ pc' = IF Watch THEN "checking" ELSE "subscribed"
    /\ watching' = TRUE
    /\ UNCHANGED <<member, owed>>

Recheck ==
    /\ pc = "checking"
    /\ IF member THEN pc' = "subscribed" /\ UNCHANGED watching ELSE pc' = "refused" /\ watching' = FALSE
    /\ UNCHANGED <<member, owed>>

TakeOut ==
    /\ member
    /\ member' = FALSE /\ owed' = TRUE
    /\ UNCHANGED <<pc, watching>>

Revalidate ==
    /\ owed
    /\ owed' = FALSE
    /\ IF watching /\ pc \in {"checking", "subscribed"}
       THEN pc' = "refused" /\ watching' = FALSE
       ELSE UNCHANGED <<pc, watching>>
    /\ UNCHANGED member

Next == StartWatching \/ Check \/ Subscribe \/ Recheck \/ TakeOut \/ Revalidate

Spec == Init /\ [][Next]_vars

(* Once the Revalidate owed is done, a subscribed socket is a member's. *)
OnlyMembersWatch == (pc = "subscribed" /\ ~owed) => member
=============================================================================
