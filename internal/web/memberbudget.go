package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/orgsettings"
)

// setMemberBudget sets what each member may spend in a month, or, for one person, what they may: their own figure, which stands
// in place of everyone's, and none to fall back to everyone's again. Past it the proxy refuses their agents' calls.
func (s *Server) setMemberBudget(w http.ResponseWriter, r *http.Request) {
	who := strings.ToLower(strings.TrimSpace(r.FormValue("who")))
	back := budgetsPath
	typed := strings.TrimSpace(r.FormValue("usd"))
	usd, err := parseBudget(typed)
	if err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	var was, now string
	err = s.orgSettings.Change(func(settings *orgsettings.Settings) {
		if who == "" {
			was, now = budgetWords(settings.MemberBudgetUSD), budgetWords(usd)
			settings.MemberBudgetUSD = usd
			return
		}
		was = memberBudgetWords(*settings, who)
		if typed == "" {
			delete(settings.MemberBudgets, who)
		} else {
			if settings.MemberBudgets == nil {
				settings.MemberBudgets = map[string]float64{}
			}
			settings.MemberBudgets[who] = usd
		}
		now = memberBudgetWords(*settings, who)
	})
	if err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	if was != now {
		whose := ""
		if who != "" {
			whose = s.personName(who) + "'s"
		}
		s.note(r, changelog.MemberBudgetSet, map[string]string{"whose": whose, "was": was, "now": now})
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// memberBudgetWords is one person's budget as the record of changes says it.
func memberBudgetWords(settings orgsettings.Settings, who string) string {
	usd, set := settings.MemberBudget(who)
	if !set {
		return "none"
	}
	return budgetWords(usd)
}

// memberBudget is what a page says of one person's month: their budget, whether it is their own or everyone's, and what they
// have spent of it.
type memberBudget struct {
	Set, Own     bool
	Limit, Spent float64
	Passed       bool
	Who          string
	Form         string
}

func (s *Server) memberBudgetOf(who string) memberBudget {
	asked, _ := s.askedThisMonth()
	return s.memberBudgetFor(who, asked.spentBy(who))
}

// memberBudgetFor is memberBudgetOf with what the person asked for this month already read, for a page of many people.
func (s *Server) memberBudgetFor(who string, spent float64) memberBudget {
	shown := memberBudget{Who: who, Form: url.QueryEscape(who), Spent: spent}
	if s.orgSettings == nil {
		return shown
	}
	settings, err := s.orgSettings.Read()
	if err != nil {
		return shown
	}
	_, shown.Own = settings.MemberBudgets[strings.ToLower(who)]
	shown.Limit, shown.Set = settings.MemberBudget(who)
	shown.Passed = shown.Set && shown.Spent >= shown.Limit
	return shown
}
