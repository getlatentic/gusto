-------------------------- MODULE ConnectorSignIn --------------------------
(***************************************************************************)
(* One person's sign-in to one app (internal/connector/connector.go): a   *)
(* sign-in finishing, its token being renewed, the person disconnecting,  *)
(* the app revoking the grant, and the server restarting.  Each action    *)
(* names the Go function it stands for.                                   *)
(*                                                                         *)
(* The app rotates refresh tokens: renewing with one gives a new one and  *)
(* makes the old one invalid, as many apps do.  An app may also give no   *)
(* refresh token at all, and then the sign-in ends when its access token  *)
(* does.                                                                   *)
(*                                                                         *)
(* Atomicity assumed:                                                      *)
(*  - a step under the sign-in's lock (lockFor) is one step, except Token,*)
(*    which holds the lock across its call to the app: that is modelled   *)
(*    as three steps with the lock held;                                   *)
(*  - Finish calls the app without the lock, then takes it to keep what   *)
(*    it got;                                                              *)
(*  - a vault write is whole, and may fail;                                *)
(*  - tokens that could not be written are held in memory (unsaved), and  *)
(*    are gone when the server restarts.                                   *)
(*                                                                         *)
(* Fixes is which fixes the code has: "disconnectLocks" is                *)
(* getlatentic/gusto#563; "finishChecksGen" is that a sign-in that        *)
(* reached the app before a disconnect is not kept after it;              *)
(* "refusedMarksBroken" is that a renewal the app refuses marks the       *)
(* sign-in broken, and a broken sign-in is not connected;                 *)
(* "expiredNeedsSignIn" is that a sign-in whose access token ran out with *)
(* no refresh token to renew it is not connected either; "holdUnsaved" is *)
(* that renewed tokens that could not be written are held in memory,      *)
(* since the app already retired the refresh token on disk.               *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Fixes,           \* a subset of AllFixes
    MaxTokens,       \* how many refresh tokens the app may give
    MaxDisconnects,  \* how many times the person may disconnect
    MaxCrashes       \* how many times the server may restart

AllFixes == {"disconnectLocks", "finishChecksGen", "refusedMarksBroken", "expiredNeedsSignIn", "holdUnsaved"}

ASSUME Fixes \subseteq AllFixes

Tokens == 1..MaxTokens

(* What is kept for the sign-in, on disk or in memory.  refresh is 0 when *)
(* the app gave none.  epoch is not in the code: it is how many           *)
(* disconnects there had been when the flow that wrote it began, so that  *)
(* a write from before a disconnect can be seen.                          *)
Kept == [present: BOOLEAN, refresh: 0..MaxTokens, live: BOOLEAN, broken: BOOLEAN,
         epoch: 0..MaxDisconnects]
Nothing == [present |-> FALSE, refresh |-> 0, live |-> FALSE, broken |-> FALSE, epoch |-> 0]

VARIABLES
    vault,       \* the sealed tokens file
    mem,         \* renewed tokens held in memory because the vault could not take them
    revoked,     \* the app has revoked the grant at some point
    valid,       \* the refresh tokens the app takes
    issued,      \* refresh tokens the app has given
    disconnects, \* disconnects so far
    gen,         \* the sign-in's generation in memory: one more for each disconnect
    lock,        \* who holds lockFor(name): "none", "token"
    finisher,    \* where Finish is: "idle", "asking", "keeping"
    finGen,      \* the generation Finish saw when the sign-in came back
    finEpoch,    \* disconnects when the sign-in came back
    finToken,    \* the refresh token the app gave Finish, or 0 for none
    renewer,     \* where Token is: "idle", "asking", "keeping", "refused"
    renRead,     \* the refresh token Token read
    renEpoch,    \* the epoch of what Token read
    renToken,    \* the refresh token the app gave Token
    up,          \* the server is running
    crashes

vars == <<vault, mem, revoked, valid, issued, disconnects, gen, lock, finisher, finGen, finEpoch, finToken,
          renewer, renRead, renEpoch, renToken, up, crashes>>

TypeOK ==
    /\ vault \in Kept /\ mem \in Kept /\ revoked \in BOOLEAN
    /\ valid \subseteq Tokens /\ issued \in 0..MaxTokens
    /\ disconnects \in 0..MaxDisconnects /\ gen \in Nat
    /\ lock \in {"none", "token"}
    /\ finisher \in {"idle", "asking", "keeping"}
    /\ finToken \in 0..MaxTokens /\ finEpoch \in 0..MaxDisconnects /\ finGen \in Nat
    /\ renewer \in {"idle", "asking", "keeping", "refused"}
    /\ renRead \in 0..MaxTokens /\ renToken \in 0..MaxTokens /\ renEpoch \in 0..MaxDisconnects
    /\ up \in BOOLEAN /\ crashes \in 0..MaxCrashes

Init ==
    /\ vault = Nothing /\ mem = Nothing /\ revoked = FALSE
    /\ valid = {} /\ issued = 0
    /\ disconnects = 0 /\ gen = 0
    /\ lock = "none"
    /\ finisher = "idle" /\ finGen = 0 /\ finEpoch = 0 /\ finToken = 0
    /\ renewer = "idle" /\ renRead = 0 /\ renEpoch = 0 /\ renToken = 0
    /\ up = TRUE /\ crashes = 0

(* What load returns: the tokens held in memory first, else the vault's. *)
Seen == IF mem.present THEN mem ELSE vault

(* Where the sign-in stands, as connector.Standing reads it: "not" signed *)
(* in, "in", or "again" for one the person has to make again.             *)
Standing ==
    IF ~Seen.present THEN "not"
    ELSE IF "refusedMarksBroken" \in Fixes /\ Seen.broken THEN "again"
    ELSE IF "expiredNeedsSignIn" \in Fixes /\ Seen.refresh = 0 /\ ~Seen.live THEN "again"
    ELSE "in"

(* Connected, as the web pages and the gateway read it. *)
Connected == Standing = "in"

-----------------------------------------------------------------------------
(* Finish: the person approved at the app and came back.  take removes the *)
(* pending sign-in, then the code is traded for tokens without the lock.   *)
FinishBegins ==
    /\ up /\ finisher = "idle"
    /\ finisher' = "asking"
    /\ finGen' = gen
    /\ finEpoch' = disconnects
    /\ UNCHANGED <<vault, mem, revoked, valid, issued, disconnects, gen, lock, finToken, renewer, renRead,
                   renEpoch, renToken, up, crashes>>

(* The app gives a refresh token, or none. *)
FinishAsks ==
    /\ finisher = "asking"
    /\ \/ /\ issued < MaxTokens
          /\ issued' = issued + 1
          /\ valid' = valid \cup {issued + 1}
          /\ finToken' = issued + 1
       \/ /\ finToken' = 0
          /\ UNCHANGED <<issued, valid>>
    /\ finisher' = "keeping"
    /\ UNCHANGED <<vault, mem, revoked, disconnects, gen, lock, finGen, finEpoch, renewer, renRead, renEpoch,
                   renToken, up, crashes>>

(* Under the lock: keep what the app gave, unless a disconnect came since *)
(* the sign-in came back.  The write may fail.  A write that works also   *)
(* drops what was held in memory.                                          *)
FinishKeeps ==
    /\ up /\ finisher = "keeping" /\ lock = "none"
    /\ IF "finishChecksGen" \in Fixes /\ finGen # gen
       THEN UNCHANGED <<vault, mem>>
       ELSE \/ /\ vault' = [present |-> TRUE, refresh |-> finToken, live |-> TRUE, broken |-> FALSE,
                            epoch |-> finEpoch]
               /\ mem' = Nothing
            \/ UNCHANGED <<vault, mem>>
    /\ finisher' = "idle"
    /\ UNCHANGED <<revoked, valid, issued, disconnects, gen, lock, finGen, finEpoch, finToken, renewer,
                   renRead, renEpoch, renToken, up, crashes>>

-----------------------------------------------------------------------------
(* Token, when the access token has run out: it holds the lock across the *)
(* call to the app.  A sign-in with no refresh token is not renewed: Token *)
(* says to sign in again without calling the app.                          *)
RenewBegins ==
    /\ up /\ renewer = "idle" /\ lock = "none"
    /\ Seen.present /\ ~Seen.live /\ Seen.refresh # 0 /\ Connected
    /\ lock' = "token"
    /\ renewer' = "asking"
    /\ renRead' = Seen.refresh
    /\ renEpoch' = Seen.epoch
    /\ UNCHANGED <<vault, mem, revoked, valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken,
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
    /\ UNCHANGED <<vault, mem, revoked, disconnects, gen, lock, finisher, finGen, finEpoch, finToken, renRead,
                   renEpoch, up, crashes>>

(* The call fails on the way: nothing changes, and the lock is let go. *)
RenewFails ==
    /\ renewer = "asking"
    /\ renewer' = "idle" /\ lock' = "none"
    /\ UNCHANGED <<vault, mem, revoked, valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken,
                   renRead, renEpoch, renToken, up, crashes>>

(* keepRenewed: vault.put of the new tokens.  When the write fails, the   *)
(* new tokens are held in memory with holdUnsaved, and without it the     *)
(* refresh token on disk, which the app retired, is all there is.          *)
RenewKeeps ==
    /\ up /\ renewer = "keeping"
    /\ LET renewed == [present |-> TRUE, refresh |-> renToken, live |-> TRUE, broken |-> FALSE, epoch |-> renEpoch]
       IN \/ /\ vault' = renewed
             /\ mem' = Nothing
          \/ /\ UNCHANGED vault
             /\ mem' = IF "holdUnsaved" \in Fixes THEN renewed ELSE mem
    /\ renewer' = "idle" /\ lock' = "none"
    /\ UNCHANGED <<revoked, valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken, renRead,
                   renEpoch, renToken, up, crashes>>

(* The app refused: with refusedMarksBroken the sign-in is kept as broken, *)
(* written to the vault, which drops what was held in memory.              *)
RenewRefused ==
    /\ up /\ renewer = "refused"
    /\ IF "refusedMarksBroken" \in Fixes /\ Seen.present
       THEN /\ vault' = [Seen EXCEPT !.broken = TRUE]
            /\ mem' = Nothing
       ELSE UNCHANGED <<vault, mem>>
    /\ renewer' = "idle" /\ lock' = "none"
    /\ UNCHANGED <<revoked, valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken, renRead,
                   renEpoch, renToken, up, crashes>>

(* The same, with the write that marks it broken failing. *)
RenewRefusedWriteFails ==
    /\ up /\ renewer = "refused"
    /\ renewer' = "idle" /\ lock' = "none"
    /\ UNCHANGED <<vault, mem, revoked, valid, issued, disconnects, gen, finisher, finGen, finEpoch, finToken,
                   renRead, renEpoch, renToken, up, crashes>>

-----------------------------------------------------------------------------
(* Disconnect: forget the tokens, those held in memory too.  Since #563 it *)
(* waits for the lock.                                                      *)
Disconnect ==
    /\ up /\ disconnects < MaxDisconnects
    /\ "disconnectLocks" \in Fixes => lock = "none"
    /\ vault' = Nothing /\ mem' = Nothing
    /\ disconnects' = disconnects + 1
    /\ gen' = gen + 1
    /\ UNCHANGED <<revoked, valid, issued, lock, finisher, finGen, finEpoch, finToken, renewer, renRead,
                   renEpoch, renToken, up, crashes>>

(* The access token runs out: of what load returns.  What is held in      *)
(* memory is newer than the vault's, whose access token ran out before    *)
(* the renewal that made it began.                                         *)
Expires ==
    /\ Seen.present /\ Seen.live
    /\ IF mem.present
       THEN /\ mem' = [mem EXCEPT !.live = FALSE]
            /\ UNCHANGED vault
       ELSE /\ vault' = [vault EXCEPT !.live = FALSE]
            /\ UNCHANGED mem
    /\ UNCHANGED <<revoked, valid, issued, disconnects, gen, lock, finisher, finGen, finEpoch, finToken,
                   renewer, renRead, renEpoch, renToken, up, crashes>>

(* The person revokes the grant at the app, or the app ends it. *)
AppRevokes ==
    /\ valid # {}
    /\ valid' = {} /\ revoked' = TRUE
    /\ UNCHANGED <<vault, mem, issued, disconnects, gen, lock, finisher, finGen, finEpoch, finToken,
                   renewer, renRead, renEpoch, renToken, up, crashes>>

-----------------------------------------------------------------------------
(* The server stops: what is in memory goes, the vault stays.  The       *)
(* generations start again, as no sign-in is under way to compare.       *)
Crash ==
    /\ up /\ crashes < MaxCrashes
    /\ up' = FALSE /\ crashes' = crashes + 1
    /\ lock' = "none" /\ finisher' = "idle" /\ renewer' = "idle" /\ gen' = 0
    /\ mem' = Nothing
    /\ UNCHANGED <<vault, revoked, valid, issued, disconnects, finGen, finEpoch, finToken, renRead,
                   renEpoch, renToken>>

Start ==
    /\ ~up /\ up' = TRUE
    /\ UNCHANGED <<vault, mem, revoked, valid, issued, disconnects, gen, lock, finisher, finGen, finEpoch,
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

(* What is kept comes from nothing begun before the last disconnect: a    *)
(* disconnect is not undone.                                               *)
DisconnectHolds ==
    /\ vault.present => vault.epoch = disconnects
    /\ mem.present => mem.epoch = disconnects

(* A sign-in the app no longer takes, or that ran out with nothing to     *)
(* renew it, stops being shown as connected, so the person is asked to    *)
(* sign in again.                                                          *)
DeadSignInIsSeen ==
    (Seen.present /\ ~Seen.live /\ Seen.refresh \notin valid)
        ~> (~Connected \/ Seen.live \/ Seen.refresh \in valid)

(* The app rotating a refresh token does not break a sign-in the person   *)
(* still has: while the server stays up and the app has revoked nothing,  *)
(* the refresh token kept is one the app takes, even when the vault could *)
(* not take the renewed one.                                               *)
RotationKeepsSignInWorking ==
    (renewer = "idle" /\ ~revoked /\ crashes = 0 /\ Connected /\ Seen.refresh # 0)
        => Seen.refresh \in valid
=============================================================================
