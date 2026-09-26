package spend

import (
	"fmt"
	"sync"
	"time"
)

// Tally is what a job, or a team in a month, has spent.
type Tally struct {
	Calls int `json:"calls"`
	// USD is the sum of every call whose cost is known. It is a floor, not a
	// total, whenever Unmeasured is above zero.
	USD          float64 `json:"usd"`
	Unmeasured   int     `json:"unmeasured"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
}

func (t *Tally) add(call Call) {
	if call.Refused != nil {
		return
	}
	t.Calls++
	if !call.Measured() {
		t.Unmeasured++
		return
	}
	t.USD += *call.USD
	t.InputTokens += valueOf(call.InputTokens)
	t.OutputTokens += valueOf(call.OutputTokens)
}

func valueOf(count *int64) int64 {
	if count == nil {
		return 0
	}
	return *count
}

// Terms are what a job was allowed when it started: the team it counts
// against, and the cap in its own budget. A nil cap is no cap; a zero cap
// allows nothing.
type Terms struct {
	Team   string
	CapUSD *float64
}

// account is one job's terms and what it has spent against them. Blind counts
// the answers the provider accepted and the proxy could not price, which is
// money a cap has no way to see.
type account struct {
	terms Terms
	tally Tally
	blind int
}

type teamMonth struct {
	team  string
	month string
}

// Ledger holds every job's spend and every team's spend this month, and
// decides whether the next call may leave. The proxy is its only writer while
// a server runs; Rebuild reads the same lines back from disk at start.
type Ledger struct {
	guard  sync.Mutex
	policy Policy
	jobs   map[string]*account
	teams  map[teamMonth]*Tally
	now    func() time.Time
}

func NewLedger(policy Policy) *Ledger {
	return &Ledger{
		policy: policy,
		jobs:   map[string]*account{},
		teams:  map[teamMonth]*Tally{},
		now:    time.Now,
	}
}

// Policy is the prices and team caps this ledger counts with.
func (l *Ledger) Policy() Policy { return l.policy }

// Begin records a job's terms. A job run again keeps what it already spent,
// because its evidence still holds those calls.
func (l *Ledger) Begin(job string, terms Terms) {
	l.guard.Lock()
	defer l.guard.Unlock()
	l.account(job).terms = terms
}

// Allow is asked before a call leaves. Nil lets it go; a refusal names the cap
// that stops it. Only calls already in flight can take a job past its cap.
func (l *Ledger) Allow(job, model string) *Refusal {
	l.guard.Lock()
	defer l.guard.Unlock()
	held := l.account(job)
	for _, check := range []func(*account, string) *Refusal{l.jobSpent, l.teamSpent, l.uncountable} {
		if refusal := check(held, model); refusal != nil {
			return refusal
		}
	}
	return nil
}

// Count adds a call that happened, or was refused, to the job and its team.
func (l *Ledger) Count(job string, call Call) {
	if !call.ToModel() {
		return
	}
	l.guard.Lock()
	defer l.guard.Unlock()
	held := l.account(job)
	held.tally.add(call)
	if call.Refused == nil && call.answered() && !call.Measured() {
		held.blind++
	}
	if held.terms.Team != "" {
		l.team(held.terms.Team, monthOf(time.Unix(call.AtUnix, 0))).add(call)
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

func (l *Ledger) team(name, month string) *Tally {
	key := teamMonth{team: name, month: month}
	tally, found := l.teams[key]
	if !found {
		tally = &Tally{}
		l.teams[key] = tally
	}
	return tally
}

func monthOf(at time.Time) string { return at.UTC().Format("2006-01") }

func (l *Ledger) jobSpent(held *account, _ string) *Refusal {
	limit := held.terms.CapUSD
	if limit == nil || held.tally.USD < *limit {
		return nil
	}
	return &Refusal{
		Cap: "job", LimitUSD: *limit, SpentUSD: held.tally.USD,
		Reason: fmt.Sprintf("this job has spent $%.4f of its $%.4f cap", held.tally.USD, *limit),
	}
}

func (l *Ledger) teamSpent(held *account, _ string) *Refusal {
	limit, capped := l.policy.teamCap(held.terms.Team)
	if !capped {
		return nil
	}
	month := monthOf(l.now())
	spent := l.team(held.terms.Team, month).USD
	if spent < limit {
		return nil
	}
	return &Refusal{
		Cap: "team", Team: held.terms.Team, LimitUSD: limit, SpentUSD: spent,
		Reason: fmt.Sprintf("team %s has spent $%.4f of its $%.4f cap for %s", held.terms.Team, spent, limit, month),
	}
}

// uncountable refuses a call a cap could not count. A cap that cannot see
// what is spent is not holding anything, so it fails closed: before the call
// when the model has no price, and after an answer that carried no usage.
func (l *Ledger) uncountable(held *account, model string) *Refusal {
	governing, found := l.governing(held)
	if !found {
		return nil
	}
	switch {
	case model != "" && !l.policy.Priced(model):
		governing.Reason = fmt.Sprintf("%s has no price in the spend policy, so the %s cap cannot count it", model, governing.Cap)
	case held.blind > 0:
		governing.Reason = fmt.Sprintf("an answer to this job carried no usage, so the %s cap cannot count what it spends", governing.Cap)
	default:
		return nil
	}
	return governing
}

// governing is the cap this job answers to, the job's own before its team's.
func (l *Ledger) governing(held *account) (*Refusal, bool) {
	if limit := held.terms.CapUSD; limit != nil {
		return &Refusal{Cap: "job", LimitUSD: *limit, SpentUSD: held.tally.USD}, true
	}
	if limit, capped := l.policy.teamCap(held.terms.Team); capped {
		spent := l.team(held.terms.Team, monthOf(l.now())).USD
		return &Refusal{Cap: "team", Team: held.terms.Team, LimitUSD: limit, SpentUSD: spent}, true
	}
	return nil, false
}
