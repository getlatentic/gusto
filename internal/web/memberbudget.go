package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/orgsettings"
	"github.com/getlatentic/gusto/pkg/spend"
)

// setMemberBudget sets what each member may spend in a month, or, for one person, what they may: their own figure, which stands
// in place of everyone's, and none to fall back to everyone's again. Past it the proxy refuses their agents' calls.
func (s *Server) setMemberBudget(w http.ResponseWriter, r *http.Request) {
	who := strings.ToLower(strings.TrimSpace(r.FormValue("who")))
	back := "/admin"
	if who != "" {
		back = "/admin/people"
	}
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
	shown := memberBudget{Who: who, Form: url.QueryEscape(who)}
	if s.orgSettings == nil {
		return shown
	}
	settings, err := s.orgSettings.Read()
	if err != nil {
		return shown
	}
	_, shown.Own = settings.MemberBudgets[strings.ToLower(who)]
	shown.Limit, shown.Set = settings.MemberBudget(who)
	if s.ledger != nil {
		shown.Spent = s.ledger.SpentThisMonth(spend.ByPerson, strings.ToLower(who))
	}
	shown.Passed = shown.Set && shown.Spent >= shown.Limit
	return shown
}
