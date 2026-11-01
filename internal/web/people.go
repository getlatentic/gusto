package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/pkg/spend"
)

// UseRoster says where the list of the organization's people is. The front door owns
// the people and writes the list, so this server shows what it says and nothing else.
// Without it the console has no page of people.
func (s *Server) UseRoster(path string) { s.rosterPath = path }

func (s *Server) hasRoster() bool { return s.rosterPath != "" }

// personRow is one member as the page says it. LastSignIn is zero for a person who
// has not been signed in yet, and Spent is nil when they spent nothing this month.
type personRow struct {
	Email      string
	Name       string
	Budget     memberBudget
	Role       string
	LastSignIn int64
	Spent      *spend.Tally
}

// inviteRow is an invitation nobody has answered.
type inviteRow struct {
	Email   string
	By      string
	Expires int64
}

// peoplePage is who is in the organization, with what each has spent this month.
func (s *Server) peoplePage(w http.ResponseWriter, r *http.Request) {
	page := map[string]any{"Title": productName + " — people", "Refused": r.URL.Query().Get("refused")}
	if s.front != nil && s.front.URL != "" {
		page["Manage"] = s.front.page("/org", nil)
		page["RoleAction"], page["Back"] = s.front.page("/org/people/role", nil), pageAddress(r)
	}
	list, err := roster.Read(s.rosterPath)
	if err != nil {
		page["Unreadable"] = err.Error()
		s.drawConsole(w, "admin-people.html", "people", page)
		return
	}
	spent, month, err := s.spentThisMonth()
	if err != nil {
		page["SpendUnreadable"] = err.Error()
	}
	page["Month"], page["Written"] = month, list.Written.Unix()
	page["People"], page["Invited"] = s.rowsOf(list, spent), invitesOf(list)
	page["Admins"] = adminsIn(list)
	page["Tokens"] = s.hasTokens()
	if s.hasOrgSettings() {
		page["Budget"] = s.budgetOf(0)
	}
	s.drawConsole(w, "admin-people.html", "people", page)
}

// pageAddress is the page a request is for, as a browser outside reads its address, for a page elsewhere to send the
// person back to.
func pageAddress(r *http.Request) string {
	scheme := "http"
	if overTLS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host + r.URL.Path
}

func adminsIn(list roster.Roster) int {
	admins := 0
	for _, member := range list.Members {
		if member.Role == "admin" {
			admins++
		}
	}
	return admins
}

func (s *Server) rowsOf(list roster.Roster, spent map[string]spend.Tally) []personRow {
	rows := make([]personRow, 0, len(list.Members))
	for _, member := range list.Members {
		row := personRow{Email: member.Email, Name: s.personName(member.Email), Role: member.Role, Budget: s.memberBudgetOf(member.Email)}
		if !member.LastSignIn.IsZero() {
			row.LastSignIn = member.LastSignIn.Unix()
		}
		if tally, found := spent[strings.ToLower(member.Email)]; found {
			row.Spent = &tally
		}
		rows = append(rows, row)
	}
	return rows
}

func invitesOf(list roster.Roster) []inviteRow {
	rows := make([]inviteRow, 0, len(list.Invited))
	for _, invited := range list.Invited {
		rows = append(rows, inviteRow{Email: invited.Email, By: invited.By, Expires: invited.Expires.Unix()})
	}
	return rows
}

// spentThisMonth is what each person who asked for work spent this month, by the
// address they signed in with, as the ledger the caps are checked against says.
func (s *Server) spentThisMonth() (map[string]spend.Tally, string, error) {
	ledger, err := s.spending()
	if err != nil {
		return nil, time.Now().UTC().Format("2006-01"), err
	}
	report := ledger.Report(ledger.ThisMonth())
	spent := map[string]spend.Tally{}
	for _, line := range s.byAsker(report.Jobs) {
		spent[strings.ToLower(line.Name)] = line.Tally
	}
	return spent, report.Month, nil
}
