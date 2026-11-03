---------------------------- MODULE Credentials -----------------------------
(***************************************************************************)
(* What one person may do on their organization's server, and what they    *)
(* carry to show it (internal/web auth.go, sent.go, device.go).  The front *)
(* door is where people are: it changes the person's role, or removes     *)
(* them, and writes the roster the server reads.  The person signs in to   *)
(* the server with a signed cookie that names their role, and approves a   *)
(* computer by the device flow, which gives the computer a token that acts *)
(* as them.  Making the token may fail.                                    *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "recheck": a cookie's person, and a computer's token's person, is     *)
(*    looked up in the roster at each request, which gives their role; a   *)
(*    person not in it is not signed in, and a computer is given no token  *)
(*    for a person who left after approving it;                            *)
(*  - "keepCode": a computer's code is spent only when its token is made,  *)
(*    so a failure lets the computer ask again.                            *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS Fixes, MaxChanges, MaxFailures

ASSUME Fixes \subseteq {"recheck", "keepCode"}

Recheck == "recheck" \in Fixes

Rank(r) == CASE r = "admin" -> 2 [] r = "runner" -> 1 [] OTHER -> 0

VARIABLES
    role,      \* the person's role in the roster: admin, runner, or out
    cookie,    \* the role the person's cookie names, or none
    code,      \* the computer's code: none, waiting, approved, taking, spent
    token,     \* the computer's token: none, or live
    wrong,     \* a request was served as more than the person is now
    lost,      \* an approval was spent and the computer was given no token
    changes, failures

vars == <<role, cookie, code, token, wrong, lost, changes, failures>>

TypeOK ==
    /\ role \in {"admin", "runner", "out"}
    /\ cookie \in {"none", "admin", "runner"}
    /\ code \in {"none", "waiting", "approved", "taking", "spent"}
    /\ token \in {"none", "live"}
    /\ wrong \in BOOLEAN /\ lost \in BOOLEAN
    /\ changes \in 0..MaxChanges /\ failures \in 0..MaxFailures

Init ==
    /\ role = "admin" /\ cookie = "none" /\ code = "none" /\ token = "none"
    /\ wrong = FALSE /\ lost = FALSE /\ changes = 0 /\ failures = 0

-----------------------------------------------------------------------------
(* The front door changes the person's role, or removes them. *)
Change ==
    /\ changes < MaxChanges
    /\ \E r \in {"admin", "runner", "out"} \ {role} : role' = r
    /\ changes' = changes + 1
    /\ UNCHANGED <<cookie, code, token, wrong, lost, failures>>

(* enter: the front door signs the person in with the role it gives them. *)
SignIn ==
    /\ role # "out"
    /\ cookie' = role
    /\ UNCHANGED <<role, code, token, wrong, lost, changes, failures>>

(* A request with the cookie is served as the role the server reads. *)
Request ==
    /\ cookie # "none"
    /\ LET served == IF Recheck THEN role ELSE cookie
       IN wrong' = (wrong \/ Rank(served) > Rank(role))
    /\ UNCHANGED <<role, cookie, code, token, lost, changes, failures>>

(* A request with the computer's token acts as the person. *)
Work ==
    /\ token = "live"
    /\ LET served == IF Recheck THEN role ELSE "runner"
       IN wrong' = (wrong \/ Rank(served) > Rank(role))
    /\ UNCHANGED <<role, cookie, code, token, lost, changes, failures>>

-----------------------------------------------------------------------------
StartDevice ==
    /\ code = "none"
    /\ code' = "waiting"
    /\ UNCHANGED <<role, cookie, token, wrong, lost, changes, failures>>

(* approveDevice, by the person signed in. *)
Approve ==
    /\ code = "waiting" /\ cookie # "none"
    /\ IF Recheck THEN role # "out" ELSE TRUE
    /\ code' = "approved"
    /\ UNCHANGED <<role, cookie, token, wrong, lost, changes, failures>>

(* pollDevice takes the approval; with keepCode it is held until the token *)
(* is made.                                                                *)
Take ==
    /\ code = "approved"
    /\ code' = IF "keepCode" \in Fixes THEN "taking" ELSE "spent"
    /\ UNCHANGED <<role, cookie, token, wrong, lost, changes, failures>>

(* CreateFor makes the token, for a person still in the organization with  *)
(* recheck.                                                                *)
Made ==
    /\ code \in {"taking", "spent"} /\ token = "none" /\ ~lost
    /\ IF Recheck THEN role # "out" ELSE TRUE
    /\ token' = "live" /\ code' = "spent"
    /\ wrong' = (wrong \/ role = "out")
    /\ UNCHANGED <<role, cookie, lost, changes, failures>>

Failed ==
    /\ code \in {"taking", "spent"} /\ token = "none" /\ ~lost /\ failures < MaxFailures
    /\ failures' = failures + 1
    /\ IF code = "taking"
       THEN code' = "approved" /\ UNCHANGED lost
       ELSE lost' = TRUE /\ UNCHANGED code
    /\ UNCHANGED <<role, cookie, token, wrong, changes>>

Next ==
    \/ Change \/ SignIn \/ Request \/ Work
    \/ StartDevice \/ Approve \/ Take \/ Made \/ Failed

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* No request is served as more than the person is now. *)
NoStaleAuthority == ~wrong

(* A computer whose approval was taken is given its token or may ask again. *)
ApprovalKept == ~lost
=============================================================================
