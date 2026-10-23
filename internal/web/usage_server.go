package web

import (
	"strconv"
	"strings"

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
	return usage.WhoOfTask(record, s.engineName, func(id string) (string, bool) {
		held, found := s.instances.Get(id)
		if !found {
			return "", false
		}
		return s.agentOf(held).Name, true
	}), true
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

// usageSpent is what a total cost as a person reads it. The calls that have no price are not in the sum, so the
// sum is said to be at least that, and a sum of nothing but unpriced calls is not shown as a figure at all.
func usageSpent(t usage.Totals) (figure, caption string) {
	switch {
	case t.Unmeasured == 0:
		return headlineUSD(t.USD), "spent"
	case t.Unmeasured == t.Calls:
		return "no price", "no price set for these models"
	}
	unpriced := strconv.FormatInt(t.Unmeasured, 10) + " calls are"
	if t.Unmeasured == 1 {
		unpriced = "1 call is"
	}
	return headlineUSD(t.USD), "spent, and " + unpriced + " not priced"
}

// cacheRate is the share of the tokens in that the provider's cache answered.
func cacheRate(cached, input int64) string {
	if input <= 0 {
		return "—"
	}
	return strconv.FormatFloat(100*float64(cached)/float64(input), 'f', 0, 64) + "%"
}
