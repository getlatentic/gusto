package web

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/usage"
)

// usageIndex is the index of model calls the usage pages read, and how far it has caught up with the files.
type usageIndex struct {
	store  usage.Store
	status func() usage.Status
}

// SetUsage gives the pages the index of model calls. A server without one answers the usage page that it records nothing.
func (s *Server) SetUsage(store usage.Store) { s.usage.store = store }

// SetUsageStatus says how far the index has caught up, so that a page shows an index still being filled as that
// and not as an empty month.
func (s *Server) SetUsageStatus(f func() usage.Status) { s.usage.status = f }

func (u usageIndex) building() usage.Status {
	if u.status == nil {
		return usage.Status{}
	}
	return u.status()
}

// UsageWho says who made a task's calls: the person who asked, the task's team and repository, the program that ran it, the
// agent a person set up when the task is a turn in one, and the conversation, or the task itself when it is in none.
func (s *Server) UsageWho(task string) (usage.Who, bool) {
	record, found := s.index.Get(task)
	if !found {
		return usage.Who{}, false
	}
	return usage.WhoOfTask(record, s.engineName, s.usageAgent), true
}

// usageAgent is the name usage gives the agent of a task's instance: a room's agent by the name it has in the room, a role's agent
// by the role, and the agent of a plain chat by its agent type, since the title of a chat is the folder it works in and says
// nothing of who answers.
func (s *Server) usageAgent(id, roomAgent string) (string, bool) {
	held, found := s.instances.Get(id)
	if !found {
		return "", false
	}
	if agent, found := s.liveRoom(held).AgentByID(roomAgent); found && roomAgent != "" {
		return agent.Name, true
	}
	if !held.Room && held.Setup == "" {
		return usage.AgentName("", "", s.engineName(held.Harness)), true
	}
	return s.agentOf(held).Name, true
}

// UsageProvider names who answers a model, as the organization's registry names its providers. A model the registry
// does not list is attributed to the key the proxy used, and to the start of its own name when it has none.
func (s *Server) UsageProvider(model, credential string) string {
	if s.registry != nil {
		document := s.registry.Document()
		for _, listed := range document.Models {
			if listed.Full() == model || (listed.Name == model && (credential == "" || listed.Provider == credential)) {
				return listed.Provider
			}
		}
		for _, listed := range document.Providers {
			if credential != "" && listed.Name == credential {
				return listed.Name
			}
		}
	}
	if credential != "" {
		return credential
	}
	provider, _, found := strings.Cut(model, "/")
	if !found {
		return ""
	}
	return provider
}

// tally is a count as a person reads it, or a dash for the count an answer did not carry.
func tally(count *int64) string {
	if count == nil {
		return "—"
	}
	return compact(*count)
}

// usageSpent is what a total cost as a person reads it. A sum of nothing but unpriced calls is not shown as a figure at all.
func usageSpent(t usage.Totals, period string) (figure, caption string) {
	if t.Unmeasured > 0 && t.Unmeasured == t.Calls {
		return "no price", "no price set for these models, " + period
	}
	return headlineUSD(t.USD), "spent, " + period
}

// usagePeriod names the dates the filter covers: both of them, one of them, or all time when it has none, since this page counts
// every call there is and not a month. A figure that names its period can be read beside the month's figures elsewhere.
func usagePeriod(values url.Values) string {
	from, to := dateWords(values.Get("from")), dateWords(values.Get("to"))
	switch {
	case from != "" && to != "":
		return from + " to " + to
	case from != "":
		return "since " + from
	case to != "":
		return "up to " + to
	}
	return "all time"
}

// dateWords is a date the filter holds as a person reads it, or nothing for what is not a date.
func dateWords(day string) string {
	when, err := time.Parse("2006-01-02", day)
	if err != nil {
		return ""
	}
	return when.Format("2 Jan 2006")
}

// unpricedNote says which calls the sum leaves out, when it leaves out some but not all, so a sum is never read as complete.
func unpricedNote(t usage.Totals) string {
	switch {
	case t.Unmeasured == 0 || t.Unmeasured == t.Calls:
		return ""
	case t.Unmeasured == 1:
		return "1 call has no price on file, so it is not in these totals."
	}
	return strconv.FormatInt(t.Unmeasured, 10) + " calls have no price on file, so they are not in these totals."
}

// cacheRate is the share of the tokens in that the provider's cache answered.
func cacheRate(cached, input int64) string {
	if input <= 0 {
		return "—"
	}
	return strconv.FormatFloat(100*float64(cached)/float64(input), 'f', 0, 64) + "%"
}
