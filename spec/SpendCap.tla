------------------------------ MODULE SpendCap ------------------------------
(***************************************************************************)
(* One person's monthly budget, as the proxy holds their tasks to it       *)
(* (pkg/spend ledger.go, cmd/gusto/wayout.go).  An admin sets the budget,  *)
(* lowers it, or sets $0.  Tasks the person asked for start and make       *)
(* calls; Ledger.Allow decides each before it leaves.  A call to a model   *)
(* with no price, and every call after an answer that carried no usage,    *)
(* cannot be counted.  The server restarts, and Rebuild gives each task    *)
(* its terms again from its spec.  Calls are counted when they end, so     *)
(* calls already in flight may take the person past the budget; the       *)
(* properties are about what Allow lets leave.                             *)
(*                                                                         *)
(* Fixes is which fixes the code has:                                      *)
(*  - "live": Allow reads the budget as it is now, not as it was when the  *)
(*    task started, so a lowered budget and a restart both hold;           *)
(*  - "governing": a person's budget refuses a call it cannot count, as a  *)
(*    task's and a team's cap do;                                          *)
(*  - "zero": a budget of $0 allows nothing, as Terms says, and is not     *)
(*    taken for no budget.                                                 *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Fixes,
    Tasks,
    Budgets,    \* the budgets an admin may set, in dollars, besides none
    MaxSpent,
    MaxCrashes

ASSUME Fixes \subseteq {"live", "governing", "zero"}

(* No budget, as a number no budget is. *)
NoBudget == 1000

VARIABLES
    budget,     \* the budget the settings hold
    started,    \* each task has begun
    terms,      \* each task's budget as the ledger holds it
    blind,      \* each task had an answer that carried no usage
    spent,      \* what the person has spent this month
    wrong,      \* a call left that the budget, as it is now, should have stopped
    crashes

vars == <<budget, started, terms, blind, spent, wrong, crashes>>

TypeOK ==
    /\ budget \in Budgets \cup {NoBudget}
    /\ started \subseteq Tasks
    /\ terms \in [Tasks -> Budgets \cup {NoBudget}]
    /\ blind \subseteq Tasks
    /\ spent \in 0..MaxSpent
    /\ wrong \in BOOLEAN /\ crashes \in 0..MaxCrashes

Init ==
    /\ budget \in Budgets \cup {NoBudget}
    /\ started = {} /\ terms = [t \in Tasks |-> NoBudget] /\ blind = {}
    /\ spent = 0 /\ wrong = FALSE /\ crashes = 0

(* MemberBudget: what the settings say the person may spend.  Without     *)
(* "zero", $0 is read as no budget.                                        *)
Read(b) == IF b = 0 /\ "zero" \notin Fixes THEN NoBudget ELSE b

(* The budget each call is held to. *)
Held(t) == IF "live" \in Fixes THEN Read(budget) ELSE terms[t]

Reached(b) == b # NoBudget /\ spent >= b

-----------------------------------------------------------------------------
SetBudget ==
    /\ \E b \in Budgets \cup {NoBudget} : budget' = b
    /\ UNCHANGED <<started, terms, blind, spent, wrong, crashes>>

(* WayOut.termsOf: the budget, read when the task starts. *)
Begin(t) ==
    /\ t \notin started
    /\ started' = started \cup {t}
    /\ terms' = [terms EXCEPT ![t] = Read(budget)]
    /\ UNCHANGED <<budget, blind, spent, wrong, crashes>>

(* Allow lets a call leave, priced or not; Count adds what it cost.  A     *)
(* budget that should stop it, as the settings hold it now, is reached,    *)
(* or cannot count the call.                                               *)
Call(t, priced) ==
    /\ t \in started /\ spent < MaxSpent
    /\ LET b == Held(t)
           countable == priced /\ t \notin blind
       IN /\ ~Reached(b)
          /\ ~("governing" \in Fixes /\ b # NoBudget /\ ~countable)
    /\ wrong' = (wrong \/ Reached(budget) \/ (budget # NoBudget /\ ~(priced /\ t \notin blind)))
    /\ spent' = IF priced THEN spent + 1 ELSE spent
    /\ blind' = IF priced THEN blind ELSE blind \cup {t}
    /\ UNCHANGED <<budget, started, terms, crashes>>

(* The server starts again, and Rebuild gives each task TermsOf its spec,  *)
(* which names no person's budget.                                         *)
Restart ==
    /\ crashes < MaxCrashes
    /\ crashes' = crashes + 1
    /\ terms' = [t \in Tasks |-> NoBudget]
    /\ UNCHANGED <<budget, started, blind, spent, wrong>>

Next ==
    \/ SetBudget \/ Restart
    \/ \E t \in Tasks : Begin(t) \/ Call(t, TRUE) \/ Call(t, FALSE)

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* No call leaves that the person's budget, as the settings hold it when   *)
(* the call is made, should stop: one past it, or one it cannot count.     *)
BudgetHolds == ~wrong
=============================================================================
