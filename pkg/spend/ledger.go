package spend

import (
	"fmt"
	"sync"
	"time"
)

// Tally is what a task, or a team in a month, has spent.
type Tally struct {
	Calls int `json:"calls"`
	// USD is the sum of every call whose cost is known. It is a floor, not a
	// total, whenever Unmeasured is above zero.
	USD        float64 `json:"usd"`
	Unmeasured int     `json:"unmeasured"`
	// Uncounted is the calls whose answer said nothing of what they used, so
	// their tokens are missing from the counts below as well as their cost.
	Uncounted    int   `json:"uncounted"`
	InputTokens  int64 `json:"inputTokens"`
	CachedTokens int64 `json:"cachedTokens"`
	OutputTokens int64 `json:"outputTokens"`
	// Refused is the calls a cap kept from leaving. They cost nothing and are
	// not in Calls.
	Refused int `json:"refused"`
}

func (t *Tally) add(call Call) {
	if call.Refused != nil {
		t.Refused++
		return
	}
	t.Calls++
	if call.Counted() {
		t.InputTokens += valueOf(call.InputTokens)
		t.CachedTokens += valueOf(call.CachedTokens)
		t.OutputTokens += valueOf(call.OutputTokens)
	} else if call.billed() {
		t.Uncounted++
	}
	switch {
	case call.Measured():
		t.USD += *call.USD
	case call.billed() || call.Counted():
		t.Unmeasured++
	}
}

func valueOf(count *int64) int64 {
	if count == nil {
		return 0
	}
	return *count
}

// Terms are what a task was allowed when it started: the team it counts
// against, the project it is reported under, and the cap in its own budget. A
// nil cap is no cap; a zero cap allows nothing.
type Terms struct {
	Team    string
	Project string
	Harness string
	CapUSD  *float64
	// Allowance names what a room's agent spends in its room, counted by the month across its turns, and AllowanceUSD is the
	// most it may spend there in one.
	Allowance    string
	AllowanceUSD *float64
}

// account is one task's terms and what it has spent against them. Blind counts
// the answers the provider accepted and the proxy could not price, which is
// money a cap has no way to see.
type account struct {
	terms Terms
	tally Tally
	blind int
}

// Scopes a call is counted under, each by the month it happened in. A day is
// counted as a scope of its own, named by its date, so a month's report can
// say how the spending fell across it.
const (
	ByJob     = "job"
	ByProject = "project"
	ByTeam    = "team"
	ByHarness = "harness"
	ByModel   = "model"
	ByDay     = "day"
	// ByAllowance is a room's agent in its room, as "<room>/<agent>".
	ByAllowance = "allowance"
)

// monthly names one tally: a task, a project or a team, in one calendar month.
type monthly struct {
	scope string
	name  string
	month string
}

// Ledger holds what every task, project and team spent in each month, and
// decides whether the next call may leave. A team's cap and the report read
// the same tallies, so they cannot disagree. The proxy is its only writer while
// a server runs; Rebuild reads the same lines back from disk at start.
type Ledger struct {
	guard  sync.Mutex
	policy Policy
	jobs   map[string]*account
	months map[monthly]*Tally
	now    func() time.Time
}

func NewLedger(policy Policy) *Ledger {
	return &Ledger{
		policy: policy,
		jobs:   map[string]*account{},
		months: map[monthly]*Tally{},
		now:    time.Now,
	}
}

// Policy is the prices and team caps this ledger counts with.
func (l *Ledger) Policy() Policy {
	l.guard.Lock()
	defer l.guard.Unlock()
	return l.policy
}

// SetPolicy changes the prices and the caps, from the next call on. What has been counted is kept: a cap that is lowered below
// what a team has spent refuses its next call, and one that is raised allows more.
func (l *Ledger) SetPolicy(policy Policy) {
	l.guard.Lock()
	defer l.guard.Unlock()
	l.policy = policy
}

// SetPrices changes what the ledger prices a model at, from the next call on. Calls it has counted keep
// the cost they were written with. The map is kept, not copied, so it is not written to afterwards.
func (l *Ledger) SetPrices(prices map[string]Price) {
	l.guard.Lock()
	defer l.guard.Unlock()
	l.policy.Prices = prices
}

// Begin records a task's terms. A task run again keeps what it already spent,
// because its record still holds those calls.
func (l *Ledger) Begin(job string, terms Terms) {
	l.guard.Lock()
	defer l.guard.Unlock()
	l.account(job).terms = terms
}

// Allow is asked before a call leaves. Nil lets it go; a refusal names the cap
// that stops it. Only calls already in flight can take a task past its cap.
func (l *Ledger) Allow(job, credential, model string) *Refusal {
	l.guard.Lock()
	defer l.guard.Unlock()
	held := l.account(job)
	if refusal := l.jobSpent(held); refusal != nil {
		return refusal
	}
	if refusal := l.teamSpent(held); refusal != nil {
		return refusal
	}
	if refusal := l.allowanceSpent(held); refusal != nil {
		return refusal
	}
	if refusal := l.organizationSpent(); refusal != nil {
		return refusal
	}
	return l.uncountable(held, credential, model)
}

// Count adds a call that happened, or was refused, to the task, its project
// and its team, in the month it happened.
func (l *Ledger) Count(job string, call Call) {
	if !call.ToModel() {
		return
	}
	l.guard.Lock()
	defer l.guard.Unlock()
	held := l.account(job)
	held.tally.add(call)
	if call.Refused == nil && call.billed() && !call.Measured() {
		held.blind++
	}
	sent := time.Unix(call.AtUnix, 0).UTC()
	month := monthOf(sent)
	for scope, name := range map[string]string{
		ByJob: job, ByProject: held.terms.Project, ByTeam: held.terms.Team,
		ByHarness: held.terms.Harness, ByModel: call.Named(), ByDay: sent.Format("2006-01-02"), ByAllowance: held.terms.Allowance,
	} {
		if scope == ByAllowance && name == "" {
			continue
		}
		l.tally(scope, name, month).add(call)
	}
}

func (l *Ledger) account(job string) *account {
	held, found := l.jobs[job]
	if !found {
		held = &account{}
		l.jobs[job] = held
	}
	return held
}

func (l *Ledger) tally(scope, name, month string) *Tally {
	key := monthly{scope: scope, name: name, month: month}
	tally, found := l.months[key]
	if !found {
		tally = &Tally{}
		l.months[key] = tally
	}
	return tally
}

func monthOf(at time.Time) string { return at.UTC().Format("2006-01") }

func (l *Ledger) jobSpent(held *account) *Refusal {
	limit := held.terms.CapUSD
	if limit == nil || held.tally.USD < *limit {
		return nil
	}
	return &Refusal{
		Cap: "job", LimitUSD: *limit, SpentUSD: held.tally.USD,
		Reason: fmt.Sprintf("this task has spent $%.4f of its $%.4f cap", held.tally.USD, *limit),
	}
}

func (l *Ledger) teamSpent(held *account) *Refusal {
	limit, capped := l.policy.teamCap(held.terms.Team)
	if !capped {
		return nil
	}
	month := monthOf(l.now())
	spent := l.tally(ByTeam, held.terms.Team, month).USD
	if spent < limit {
		return nil
	}
	return &Refusal{
		Cap: "team", Team: held.terms.Team, LimitUSD: limit, SpentUSD: spent,
		Reason: fmt.Sprintf("team %s has spent $%.4f of its $%.4f cap for %s", held.terms.Team, spent, limit, month),
	}
}

func (l *Ledger) allowanceSpent(held *account) *Refusal {
	limit := held.terms.AllowanceUSD
	if limit == nil || held.terms.Allowance == "" {
		return nil
	}
	month := monthOf(l.now())
	spent := l.tally(ByAllowance, held.terms.Allowance, month).USD
	if spent < *limit {
		return nil
	}
	return &Refusal{
		Cap: "allowance", LimitUSD: *limit, SpentUSD: spent,
		Reason: fmt.Sprintf("this agent has spent $%.4f of its $%.4f allowance in this room for %s", spent, *limit, month),
	}
}

// SpentThisMonth is what one scope's name has spent in the current month, such as an agent in its room under ByAllowance.
func (l *Ledger) SpentThisMonth(scope, name string) float64 {
	l.guard.Lock()
	defer l.guard.Unlock()
	if tally, found := l.months[monthly{scope: scope, name: name, month: monthOf(l.now())}]; found {
		return tally.USD
	}
	return 0
}

// organizationSpent is what every task has spent this month, against the cap on the whole server. It is summed from the tasks,
// which is the sum the report gives as its total, so the cap and the report cannot disagree.
func (l *Ledger) organizationSpent() *Refusal {
	limit, capped := l.policy.organizationCap()
	if !capped {
		return nil
	}
	month := monthOf(l.now())
	spent := 0.0
	for key, tally := range l.months {
		if key.scope == ByJob && key.month == month {
			spent += tally.USD
		}
	}
	if spent < limit {
		return nil
	}
	return &Refusal{
		Cap: "organisation", LimitUSD: limit, SpentUSD: spent,
		Reason: fmt.Sprintf("this organization has spent $%.4f of its $%.4f cap for %s", spent, limit, month),
	}
}

// uncountable refuses a call a cap could not count. A cap that cannot see
// what is spent is not holding anything, so it fails closed: before the call
// when the model has no price, and after an answer that carried no usage.
func (l *Ledger) uncountable(held *account, credential, model string) *Refusal {
	governing, found := l.governing(held)
	if !found {
		return nil
	}
	switch {
	case model != "" && !l.policy.Priced(credential, model):
		governing.Reason = fmt.Sprintf("%s has no price in the spend policy, so the %s cap cannot count it", model, governing.Cap)
	case held.blind > 0:
		governing.Reason = fmt.Sprintf("an answer to this task carried no usage, so the %s cap cannot count what it spends", governing.Cap)
	default:
		return nil
	}
	return governing
}

// governing is the cap this task answers to, the task's own before its team's.
func (l *Ledger) governing(held *account) (*Refusal, bool) {
	if limit := held.terms.CapUSD; limit != nil {
		return &Refusal{Cap: "job", LimitUSD: *limit, SpentUSD: held.tally.USD}, true
	}
	if limit, capped := l.policy.teamCap(held.terms.Team); capped {
		spent := l.tally(ByTeam, held.terms.Team, monthOf(l.now())).USD
		return &Refusal{Cap: "team", Team: held.terms.Team, LimitUSD: limit, SpentUSD: spent}, true
	}
	if limit, capped := l.policy.organizationCap(); capped {
		return &Refusal{Cap: "organisation", LimitUSD: limit}, true
	}
	return nil, false
}
