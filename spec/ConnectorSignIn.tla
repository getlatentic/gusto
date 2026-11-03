-------------------------- MODULE ConnectorSignIn --------------------------
(***************************************************************************)
(* One person's sign-in to one app (internal/connector/connector.go): a   *)
(* sign-in finishing, its token being renewed, the person disconnecting,  *)
(* the app revoking the grant, and the server restarting.  Each action    *)
(* names the Go function it stands for.                                   *)
(*                                                                         *)
(* The app rotates refresh tokens: renewing with one gives a new one and  *)
(* makes the old one invalid, as many apps do.                            *)
(*                                                                         *)
(* Atomicity assumed:                                                      *)
(*  - a step under the sign-in's lock (lockFor) is one step, except Token,*)
(*    which holds the lock across its call to the app: that is modelled   *)
(*    as three steps with the lock held;                                   *)
(*  - Finish calls the app without the lock, then takes it to keep what   *)
(*    it got;                                                              *)
(*  - a vault write is whole, and may fail.                                *)
(*                                                                         *)
(* Fixes is which fixes the code has: "disconnectLocks" is                *)
(* getlatentic/gusto#563; "finishChecksGen" is that a sign-in that        *)
(* reached the app before a disconnect is not kept after it;              *)
(* "refusedMarksBroken" is that a renewal the app refuses marks the       *)
(* sign-in broken, and a broken sign-in is not connected.                 *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Fixes,           \* a subset of {"disconnectLocks", "finishChecksGen", "refusedMarksBroken"}
    MaxTokens,       \* how many refresh tokens the app may give
    MaxDisconnects,  \* how many times the person may disconnect
    MaxCrashes       \* how many times the server may restart

ASSUME Fixes \subseteq {"disconnectLocks", "finishChecksGen", "refusedMarksBroken"}

Tokens == 1..MaxTokens

(* What the vault keeps for the sign-in.  epoch is not in the code: it is *)
(* how many disconnects there had been when the flow that wrote it began, *)
(* so that a write from before a disconnect can be seen.                  *)
Kept == [present: BOOLEAN, refresh: 0..MaxTokens, live: BOOLEAN, broken: BOOLEAN,
         epoch: 0..MaxDisconnects]
Nothing == [present |-> FALSE, refresh |-> 0, live |-> FALSE, broken |-> FALSE, epoch |-> 0]

VARIABLES
    vault,       \* the sealed tokens file
    valid,       \* the refresh tokens the app takes
    issued,      \* refresh tokens the app has given
    disconnects, \* disconnects so far
    gen,         \* the sign-in's generation in memory: one more for each disconnect
    lock,        \* who holds lockFor(name): "none", "token"
    finisher,    \* where Finish is: "idle", "asking", "keeping"
    finGen,      \* the generation Finish saw when the sign-in came back
    finEpoch,    \* disconnects when the sign-in came back
    finToken,    \* the refresh token the app gave Finish
    renewer,     \* where Token is: "idle", "asking", "keeping", "refused"
    renRead,     \* the refresh token Token read
    renEpoch,    \* the epoch of what Token read
    renToken,    \* the refresh token the app gave Token
    up,          \* the server is running
    crashes

vars == <<vault, valid, issued, disconnects, gen, lock, finisher, finGen, finEpoch, finToken,
          renewer, renRead, renEpoch, renToken, up, crashes>>

TypeOK ==
    /\ vault \in Kept
    /\ valid \subseteq Tokens /\ issued \in 0..MaxTokens
    /\ disconnects \in 0..MaxDisconnects /\ gen \in Nat
    /\ lock \in {"none", "token"}
    /\ finisher \in {"idle", "asking", "keeping"}
    /\ finToken \in 0..MaxTokens /\ finEpoch \in 0..MaxDisconnects /\ finGen \in Nat
    /\ renewer \in {"idle", "asking", "keeping", "refused"}
    /\ renRead \in 0..MaxTokens /\ renToken \in 0..MaxTokens /\ renEpoch \in 0..MaxDisconnects
    /\ up \in BOOLEAN /\ crashes \in 0..MaxCrashes

Init ==
    /\ vault = Nothing
    /\ valid = {} /\ issued = 0
    /\ disconnects = 0 /\ gen = 0
    /\ lock = "none"
    /\ finisher = "idle" /\ finGen = 0 /\ finEpoch = 0 /\ finToken = 0
    /\ renewer = "idle" /\ renRead = 0 /\ renEpoch = 0 /\ renToken = 0
    /\ up = TRUE /\ crashes = 0

(* Connected, as the web pages and the gateway read it. *)
Connected == vault.present /\ ~("refusedMarksBroken" \in Fixes /\ vault.broken)

-----------------------------------------------------------------------------
(* Finish: the person approved at the app and came back.  take removes the *)
(* pending sign-in, then the code is traded for tokens without the lock.   *)
FinishBegins ==
    /\ up /\ finisher = "idle"
    /\ finisher' = "asking"
    /\ finGen' = gen
    /\ finEpoch' = disconnects
    /\ UNCHANGED <<vault, valid, issued, disconnects, gen, lock, finToken, renewer, renRead,
                   renEpoch, renToken, up, crashes>>

FinishAsks ==
    /\ finisher = "asking" /\ issued < MaxTokens
    /\ issued' = issued + 1
    /\ valid' = valid \cup {issued + 1}
    /\ finToken' = issued + 1
    /\ finisher' = "keeping"
    /\ UNCHANGED <<vault, disconnects, gen, lock, finGen, finEpoch, renewer, renRead, renEpoch,
                   renToken, up, crashes>>

(* Under the lock: keep what the app gave, unless a disconnect came since *)
(* the sign-in came back.  The write may fail.                            *)
FinishKeeps ==
    /\ up /\ finisher = "keeping" /\ lock = "none"
    /\ IF "finishChecksGen" \in Fixes /\ finGen # gen
       THEN UNCHANGED vault
       ELSE \/ vault' = [present |-> TRUE, refresh |-> finToken, live |-> TRUE, broken |-> FALSE,
                         epoch |-> finEpoch]
            \/ UNCHANGED vault
    /\ finisher' = "idle"
    /\ UNCHANGED <<valid, issued, disconnects, gen, lock, finGen, finEpoch, finToken, renewer,
                   renRead, renEpoch, renToken, up, crashes>>

-----------------------------------------------------------------------------
(* Token, when the access token has run out: it holds the lock across the *)
(* call to the app.                                                        *)
RenewBegins ==
    /\ up /\ renewer = "idle" /\ lock = "none"
    /\ vault.present /\ ~vault.live /\ Connected
    /\ lock' = "token"
    /\ renewer' = "asking"
    /\ renRead' = vault.refresh
    /\ renEpoch' = vault.epoch
    /\ UNCHANGED <<vault, valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken,
                   renToken, up, crashes>>

(* The app renews a token it takes, rotating it, and refuses one it does  *)
(* not: invalid_grant.                                                     *)
RenewAsks ==
    /\ renewer = "asking"
    /\ IF renRead \in valid
       THEN /\ issued < MaxTokens
            /\ issued' = issued + 1
            /\ valid' = (valid \ {renRead}) \cup {issued + 1}
            /\ renToken' = issued + 1
            /\ renewer' = "keeping"
       ELSE /\ renewer' = "refused"
            /\ UNCHANGED <<issued, valid, renToken>>
    /\ UNCHANGED <<vault, disconnects, gen, lock, finisher, finGen, finEpoch, finToken, renRead,
                   renEpoch, up, crashes>>

(* The call fails on the way: nothing changes, and the lock is let go. *)
RenewFails ==
    /\ renewer = "asking"
    /\ renewer' = "idle" /\ lock' = "none"
    /\ UNCHANGED <<vault, valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken,
                   renRead, renEpoch, renToken, up, crashes>>

(* vault.put of the new tokens, which may fail. *)
RenewKeeps ==
    /\ up /\ renewer = "keeping"
    /\ \/ vault' = [present |-> TRUE, refresh |-> renToken, live |-> TRUE, broken |-> FALSE,
                    epoch |-> renEpoch]
       \/ UNCHANGED vault
    /\ renewer' = "idle" /\ lock' = "none"
    /\ UNCHANGED <<valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken, renRead,
                   renEpoch, renToken, up, crashes>>

(* The app refused: with refusedMarksBroken the sign-in is kept as broken. *)
RenewRefused ==
    /\ up /\ renewer = "refused"
    /\ IF "refusedMarksBroken" \in Fixes /\ vault.present
       THEN vault' = [vault EXCEPT !.broken = TRUE]
       ELSE UNCHANGED vault
    /\ renewer' = "idle" /\ lock' = "none"
    /\ UNCHANGED <<valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken, renRead,
                   renEpoch, renToken, up, crashes>>

(* The same, with the write that marks it broken failing. *)
RenewRefusedWriteFails ==
    /\ up /\ renewer = "refused"
    /\ renewer' = "idle" /\ lock' = "none"
    /\ UNCHANGED <<vault, valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken,
                   renRead, renEpoch, renToken, up, crashes>>

-----------------------------------------------------------------------------
(* Disconnect: forget the tokens.  Since #563 it waits for the lock.      *)
Disconnect ==
    /\ up /\ disconnects < MaxDisconnects
    /\ "disconnectLocks" \in Fixes => lock = "none"
    /\ vault' = Nothing
    /\ disconnects' = disconnects + 1
    /\ gen' = gen + 1
    /\ UNCHANGED <<valid, issued, lock, finisher, finGen, finEpoch, finToken, renewer, renRead,
                   renEpoch, renToken, up, crashes>>

(* The access token runs out. *)
Expires ==
    /\ vault.present /\ vault.live
    /\ vault' = [vault EXCEPT !.live = FALSE]
    /\ UNCHANGED <<valid, issued, disconnects, gen, lock, finisher, finGen, finEpoch, finToken,
                   renewer, renRead, renEpoch, renToken, up, crashes>>

(* The person revokes the grant at the app, or the app ends it. *)
AppRevokes ==
    /\ valid # {}
    /\ valid' = {}
    /\ UNCHANGED <<vault, issued, disconnects, gen, lock, finisher, finGen, finEpoch, finToken,
                   renewer, renRead, renEpoch, renToken, up, crashes>>

-----------------------------------------------------------------------------
(* The server stops: what is in memory goes, the vault stays.  The       *)
(* generations start again, as no sign-in is under way to compare.       *)
Crash ==
    /\ up /\ crashes < MaxCrashes
    /\ up' = FALSE /\ crashes' = crashes + 1
    /\ lock' = "none" /\ finisher' = "idle" /\ renewer' = "idle" /\ gen' = 0
    /\ UNCHANGED <<vault, valid, issued, disconnects, finGen, finEpoch, finToken, renRead,
                   renEpoch, renToken>>

Start ==
    /\ ~up /\ up' = TRUE
    /\ UNCHANGED <<vault, valid, issued, disconnects, gen, lock, finisher, finGen, finEpoch,
                   finToken, renewer, renRead, renEpoch, renToken, crashes>>

Next ==
    \/ FinishBegins \/ FinishAsks \/ FinishKeeps
    \/ RenewBegins \/ RenewAsks \/ RenewFails \/ RenewKeeps \/ RenewRefused \/ RenewRefusedWriteFails
    \/ Disconnect \/ Expires \/ AppRevokes
    \/ Crash \/ Start

(* Somebody keeps using the app, so a run-out token is renewed in time,  *)
(* and a call under way ends.  The app answers in the end, however often  *)
(* a call fails on the way, and the disk takes a write in the end: strong *)
(* fairness, since each failure starts the renewal again.                 *)
Fairness ==
    /\ WF_vars(RenewBegins) /\ SF_vars(RenewAsks) /\ WF_vars(RenewKeeps) /\ SF_vars(RenewRefused)
    /\ WF_vars(FinishAsks) /\ WF_vars(FinishKeeps) /\ WF_vars(Start)

Spec == Init /\ [][Next]_vars /\ Fairness

-----------------------------------------------------------------------------
(* What must hold. *)

(* What the vault holds comes from nothing begun before the last          *)
(* disconnect: a disconnect is not undone.                                *)
DisconnectHolds == vault.present => vault.epoch = disconnects

(* A sign-in the app no longer takes stops being shown as connected,     *)
(* so the person is asked to sign in again.                               *)
DeadSignInIsSeen ==
    (vault.present /\ ~vault.live /\ vault.refresh \notin valid)
        ~> (~Connected \/ vault.live \/ vault.refresh \in valid)
=============================================================================
