-------------------------------- MODULE WayOut -------------------------------
(***************************************************************************)
(* A task's ways out of its sandbox: the tool gateway (pkg/gateway) and    *)
(* the model proxy's tokens (pkg/proxy tokens.go, internal/runtoken).      *)
(*                                                                         *)
(* The gateway reads a server's tools when a request first asks; the      *)
(* request may be cancelled, or the server down, and the server comes up. *)
(* Two calls share one session with the server, which the server may      *)
(* lose.  The proxy mints a task's token, and mints it again when the run *)
(* restarts with another credential.  A run's forwarder trades its token  *)
(* for a new one near its end, from calls made at once, and the run ends. *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "reread": a read is not ended by its request, and a server that      *)
(*    could not be read is read again later;                               *)
(*  - "session": a lost session ends only if it is still the one in use;  *)
(*  - "mint": minting again keeps the token and takes the new credential;  *)
(*  - "live": a run that has ended is not given a new token;               *)
(*  - "flight": one trade at a time.                                       *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes

ASSUME Fixes \subseteq {"reread", "session", "mint", "live", "flight"}

VARIABLES
    server,    \* the tool server is up
    catalog,   \* its tools: unread, read, or broken
    session,   \* the session in use, by number, or 0
    starts,    \* how many sessions were started
    losses,    \* how many times the server lost one
    holding,   \* the session each of two calls is in, 0 when idle
    given,     \* the credential the task's run was started with now
    ticket,    \* the credential its token is good for
    running,   \* the run is running
    late,      \* a token was minted for a run that had ended
    trading    \* how many trades of the run's token are under way

vars == <<server, catalog, session, starts, losses, holding, given, ticket, running, late, trading>>

Calls == {1, 2}

TypeOK ==
    /\ server \in BOOLEAN /\ catalog \in {"unread", "read", "broken"}
    /\ session \in 0..3 /\ starts \in 0..3 /\ losses \in 0..1
    /\ holding \in [Calls -> 0..3]
    /\ given \in {"a", "b"} /\ ticket \in {"a", "b"}
    /\ running \in BOOLEAN /\ late \in BOOLEAN /\ trading \in 0..2

Init ==
    /\ server = FALSE /\ catalog = "unread"
    /\ session = 0 /\ starts = 0 /\ losses = 0 /\ holding = [c \in Calls |-> 0]
    /\ given = "a" /\ ticket = "a" /\ running = TRUE /\ late = FALSE /\ trading = 0

Gateway == <<server, catalog, session, starts, losses, holding>>
Proxy == <<given, ticket, running, late, trading>>

-----------------------------------------------------------------------------
Up == ~server /\ server' = TRUE /\ UNCHANGED <<catalog, session, starts, losses, holding>> /\ UNCHANGED Proxy

(* A read while the server is down, or by a request that is cancelled, fails. *)
ReadFails ==
    /\ catalog = "unread" \/ (catalog = "broken" /\ "reread" \in Fixes)
    /\ ~server \/ "reread" \notin Fixes
    /\ catalog' = "broken"
    /\ UNCHANGED <<server, session, starts, losses, holding>> /\ UNCHANGED Proxy

Read ==
    /\ catalog = "unread" \/ (catalog = "broken" /\ "reread" \in Fixes)
    /\ server
    /\ catalog' = "read"
    /\ UNCHANGED <<server, session, starts, losses, holding>> /\ UNCHANGED Proxy

(* A call starts a session when there is none, and calls in it. *)
Begin(c) ==
    /\ holding[c] = 0 /\ starts < 3
    /\ IF session = 0
       THEN session' = starts + 1 /\ starts' = starts + 1 /\ holding' = [holding EXCEPT ![c] = starts + 1]
       ELSE UNCHANGED <<session, starts>> /\ holding' = [holding EXCEPT ![c] = session]
    /\ UNCHANGED <<server, catalog, losses>> /\ UNCHANGED Proxy

(* The server lost the call's session; the call ends it and goes again.    *)
Lost(c) ==
    /\ holding[c] # 0 /\ losses = 0 /\ session = holding[c]
    /\ losses' = 1 /\ session' = 0 /\ holding' = [holding EXCEPT ![c] = 0]
    /\ UNCHANGED <<server, catalog, starts>> /\ UNCHANGED Proxy

(* A call told late that its own session is gone, after another started a *)
(* new one.                                                                *)
LostLate(c) ==
    /\ holding[c] # 0 /\ session # 0 /\ session # holding[c]
    /\ session' = IF "session" \in Fixes THEN session ELSE 0
    /\ holding' = [holding EXCEPT ![c] = 0]
    /\ UNCHANGED <<server, catalog, starts, losses>> /\ UNCHANGED Proxy

Done(c) ==
    /\ holding[c] # 0 /\ holding[c] = session
    /\ holding' = [holding EXCEPT ![c] = 0]
    /\ UNCHANGED <<server, catalog, session, starts, losses>> /\ UNCHANGED Proxy

-----------------------------------------------------------------------------
(* The run restarts with another credential, and its token is minted again. *)
Restart ==
    /\ running /\ given = "a"
    /\ given' = "b"
    /\ ticket' = IF "mint" \in Fixes THEN "b" ELSE ticket
    /\ UNCHANGED <<running, late, trading>> /\ UNCHANGED Gateway

StartTrade ==
    /\ trading < 2
    /\ trading = 0 \/ "flight" \notin Fixes
    /\ trading' = trading + 1
    /\ UNCHANGED <<given, ticket, running, late>> /\ UNCHANGED Gateway

Traded ==
    /\ trading > 0
    /\ trading' = trading - 1
    /\ IF running \/ "live" \in Fixes THEN UNCHANGED late ELSE late' = TRUE
    /\ UNCHANGED <<given, ticket, running>> /\ UNCHANGED Gateway

End ==
    /\ running
    /\ running' = FALSE
    /\ UNCHANGED <<given, ticket, late, trading>> /\ UNCHANGED Gateway

Next ==
    \/ Up \/ ReadFails \/ Read
    \/ \E c \in Calls : Begin(c) \/ Lost(c) \/ LostLate(c) \/ Done(c)
    \/ Restart \/ StartTrade \/ Traded \/ End

Spec == Init /\ [][Next]_vars

FairSpec == Spec /\ WF_vars(Up) /\ WF_vars(Read)

-----------------------------------------------------------------------------
(* A server that is up has its tools read. *)
UpIsRead == <>(catalog = "read")

(* A session is started again only when the server lost one. *)
NoNeedlessStart == starts <= 1 + losses

(* A task's token is good for the credential its run was started with. *)
TokenIsCurrent == ticket = given

(* No token is minted for a run that has ended. *)
NoneAfterEnd == ~late

(* One trade of a run's token at a time. *)
OneTrade == trading <= 1
=============================================================================
