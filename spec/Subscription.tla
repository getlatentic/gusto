---------------------------- MODULE Subscription ----------------------------
(***************************************************************************)
(* One organization's subscription as Stripe changes it and as gusto       *)
(* keeps it (internal/cloud stripeWebhook, RecordSubscription).  Stripe    *)
(* makes an event for each change, stamped with the second it was made,    *)
(* and sends it until it is answered, in any order, to webhook handlers   *)
(* that run at once.  The row keeps one version of the subscription and    *)
(* the stamp it came with; an event stamped earlier than the row changes   *)
(* nothing.  A change is its place in Stamps, so a later change has a      *)
(* larger number.                                                          *)
(*                                                                         *)
(* Fixes is which fixes the code has: "tieFetch" is that an event stamped  *)
(* the same second as the row, with a different version, is not kept:     *)
(* the handler asks Stripe for the subscription as it is, stamped with    *)
(* the second Stripe answered in, and keeps that by the same rule.         *)
(***************************************************************************)
EXTENDS Naturals, Sequences

CONSTANTS
    Fixes,     \* a subset of {"tieFetch"}
    Stamps,    \* the second each change is made in, in order
    Handlers,  \* the webhook handlers
    MaxSecond  \* how far Stripe's clock goes

ASSUME Fixes \subseteq {"tieFetch"}

(* Checkout's created and updated events, made in one second, then a change later. *)
CheckoutStamps == <<1, 1, 1, 2>>

Last == Len(Stamps)
Changes == 1..Last

VARIABLES
    made,     \* how many changes Stripe has made; the subscription is the last
    clock,    \* Stripe's clock, in whole seconds
    held,     \* the row: the change it holds, and the second it is stamped
    answered, \* the events Stripe stops sending
    handler   \* each handler: idle, or what it is keeping, for which event

vars == <<made, clock, held, answered, handler>>

Version == [change : 0..Last, at : 0..MaxSecond]

TypeOK ==
    /\ made \in Changes /\ clock \in 0..MaxSecond
    /\ held \in Version /\ answered \subseteq Changes
    /\ handler \in [Handlers -> [st : {"idle", "keep", "fetch"}, v : Version, event : 0..Last]]

Idle == [st |-> "idle", v |-> [change |-> 0, at |-> 0], event |-> 0]

Init ==
    /\ made = 1 /\ clock = Stamps[1]
    /\ held = [change |-> 0, at |-> 0]
    /\ answered = {}
    /\ handler = [h \in Handlers |-> Idle]

-----------------------------------------------------------------------------
(* Stripe makes the next change, in its second. *)
Change ==
    /\ made < Last /\ clock <= Stamps[made + 1]
    /\ made' = made + 1 /\ clock' = Stamps[made + 1]
    /\ UNCHANGED <<held, answered, handler>>

Tick ==
    /\ clock < MaxSecond /\ IF made = Last THEN TRUE ELSE clock < Stamps[made + 1]
    /\ clock' = clock + 1
    /\ UNCHANGED <<made, held, answered, handler>>

(* Stripe sends an event, again or for the first time, to a free handler. *)
Deliver(h, e) ==
    /\ handler[h].st = "idle" /\ e <= made
    /\ handler' = [handler EXCEPT ![h] = [st |-> "keep", v |-> [change |-> e, at |-> Stamps[e]], event |-> e]]
    /\ UNCHANGED <<made, clock, held, answered>>

Done(h) == /\ handler' = [handler EXCEPT ![h] = Idle]
           /\ answered' = answered \cup {handler[h].event}

(* RecordSubscription, in its transaction. *)
Keep(h) ==
    /\ handler[h].st = "keep"
    /\ LET v == handler[h].v
       IN CASE held.change = 0 \/ v.at > held.at -> held' = v /\ Done(h)
            [] v.at < held.at \/ v.change = held.change -> UNCHANGED held /\ Done(h)
            [] "tieFetch" \in Fixes ->
                 /\ handler' = [handler EXCEPT ![h].st = "fetch"]
                 /\ UNCHANGED <<held, answered>>
            [] OTHER -> held' = v /\ Done(h)
    /\ UNCHANGED <<made, clock>>

(* Stripe answers with the subscription as it is, and the second it answers in. *)
Fetch(h) ==
    /\ handler[h].st = "fetch"
    /\ handler' = [handler EXCEPT ![h].st = "keep", ![h].v = [change |-> made, at |-> clock]]
    /\ UNCHANGED <<made, clock, held, answered>>

Next ==
    \/ Change \/ Tick
    \/ \E h \in Handlers : (\E e \in Changes : Deliver(h, e)) \/ Keep(h) \/ Fetch(h)

(* Stripe sends each event until it is answered, handlers finish, and time passes. *)
Fairness ==
    /\ \A e \in Changes : SF_vars(e \notin answered /\ \E h \in Handlers : Deliver(h, e))
    /\ \A h \in Handlers : WF_vars(Keep(h)) /\ WF_vars(Fetch(h))
    /\ WF_vars(Change) /\ WF_vars(Tick)

Spec == Init /\ [][Next]_vars /\ Fairness

-----------------------------------------------------------------------------
(* The row never goes back to an older version of the subscription. *)
NeverOlder == [][held'.change >= held.change]_held

(* Once Stripe stops changing it, the row comes to hold the subscription as it is. *)
Converges == <>[](held.change = Last)
=============================================================================
