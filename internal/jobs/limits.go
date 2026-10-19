package jobs

import "sort"

// Limits are how much one person or team may run at once. Zero is no limit.
// Slots is how many tasks the whole server runs at once, which the queue sets.
type Limits struct {
	Person int
	Team   int
	Slots  int
}

// SetLimits sets the per-person and per-team limits. A task over its limit
// stays queued, and is taken when one of that person's or team's tasks ends.
func (i *Index) SetLimits(l Limits) {
	i.guard.Lock()
	defer i.guard.Unlock()
	l.Slots = i.limits.Slots
	i.limits = l
}

// Limits are the limits now.
func (i *Index) Limits() Limits {
	i.guard.Lock()
	defer i.guard.Unlock()
	return i.limits
}

func (i *Index) setSlots(slots int) {
	i.guard.Lock()
	defer i.guard.Unlock()
	i.limits.Slots = slots
}

// Reason is why a queued task has not started.
type Reason int

const (
	// Starting is a task nothing holds back: a free worker takes it next.
	Starting Reason = iota
	// AgentBusy is a task for an agent that is still on another message.
	AgentBusy
	// PersonAtLimit is a task whose person already runs as many as they may.
	PersonAtLimit
	// TeamAtLimit is a task whose team already runs as many as it may.
	TeamAtLimit
	// OrgAtLimit is a task whose organization already runs as many as its quota
	// allows.
	OrgAtLimit
	// PoolFull is a task that may run, with every worker busy.
	PoolFull
)

// Wait is where a queued task stands and what it waits for.
type Wait struct {
	Reason Reason
	// Position counts the queued tasks from the front, starting at 1.
	Position int
	// Running and Limit are the count and the limit that hold the task, for
	// PersonAtLimit, TeamAtLimit and PoolFull.
	Running, Limit int
	// Who is the person, or the team, that is at their limit.
	Who string
}

// running is what is running now, counted for the limits.
type running struct {
	total    int
	agents   map[string]bool
	byPerson map[string]int
	byTeam   map[string]int
}

func (i *Index) running() running {
	now := running{agents: map[string]bool{}, byPerson: map[string]int{}, byTeam: map[string]int{}}
	for _, record := range i.records {
		if record.State != Running {
			continue
		}
		now.total++
		if record.Instance != "" {
			now.agents[record.Instance] = true
		}
		now.byPerson[record.Asked]++
		if record.Team != "" {
			now.byTeam[record.Team]++
		}
	}
	return now
}

// hold is why the record cannot start now, and whether anything does. An
// agent's threads share one workspace, so a turn waits while another turn of
// the same agent runs; a job that is not a turn has no agent and waits for none.
func (i *Index) hold(record Record, now running) (Wait, bool) {
	switch {
	case record.Instance != "" && now.agents[record.Instance]:
		return Wait{Reason: AgentBusy}, true
	case i.limits.Person > 0 && record.Asked != "" && now.byPerson[record.Asked] >= i.limits.Person:
		return Wait{Reason: PersonAtLimit, Running: now.byPerson[record.Asked], Limit: i.limits.Person, Who: record.Asked}, true
	case i.limits.Team > 0 && record.Team != "" && now.byTeam[record.Team] >= i.limits.Team:
		return Wait{Reason: TeamAtLimit, Running: now.byTeam[record.Team], Limit: i.limits.Team, Who: record.Team}, true
	case i.orgFull(now.total):
		limit, _ := i.orgLimit()
		return Wait{Reason: OrgAtLimit, Running: now.total, Limit: limit}, true
	case i.limits.Slots > 0 && now.total >= i.limits.Slots:
		return Wait{Reason: PoolFull, Running: now.total, Limit: i.limits.Slots}, true
	}
	return Wait{Reason: Starting}, false
}

// WaitOf says where a queued task stands. It answers false for a task that is
// not queued.
func (i *Index) WaitOf(id string) (Wait, bool) {
	i.guard.Lock()
	defer i.guard.Unlock()
	record, found := i.records[id]
	if !found || record.State != Queued {
		return Wait{}, false
	}
	wait, _ := i.hold(record, i.running())
	for _, other := range i.records {
		if other.State == Queued && other.Seq < record.Seq {
			wait.Position++
		}
	}
	wait.Position++
	return wait, true
}

// Waiting is every queued task, front first, with what holds each.
func (i *Index) Waiting() []Waiting {
	i.guard.Lock()
	defer i.guard.Unlock()
	now := i.running()
	var queued []Record
	for _, record := range i.records {
		if record.State == Queued {
			queued = append(queued, record)
		}
	}
	sort.Slice(queued, func(a, b int) bool { return queued[a].Seq < queued[b].Seq })
	out := make([]Waiting, len(queued))
	for n, record := range queued {
		wait, _ := i.hold(record, now)
		wait.Position = n + 1
		out[n] = Waiting{Record: record, Wait: wait}
	}
	return out
}

// Waiting is a queued task and what holds it.
type Waiting struct {
	Record Record
	Wait   Wait
}
