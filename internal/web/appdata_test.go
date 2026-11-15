package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

// data is a console page's data as the app reads it from /app/api, decoded into into.
func (s *site) data(t *testing.T, path string, into any) {
	t.Helper()
	answer := s.ask(t, "GET", "/app/api"+path, nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the data of %s answered %d: %s", path, answer.Code, answer.Body.String())
	}
	if err := json.Unmarshal(answer.Body.Bytes(), into); err != nil {
		t.Fatalf("the data of %s is not JSON: %v", path, err)
	}
}

// dataText is a console page's data as text, for a test that looks for what is in it.
func (s *site) dataText(t *testing.T, path string) string {
	t.Helper()
	answer := s.ask(t, "GET", "/app/api"+path, nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the data of %s answered %d: %s", path, answer.Code, answer.Body.String())
	}
	return answer.Body.String()
}

// peopleJSON is the People page's data.
type peopleJSON struct {
	People  []personJSON
	Invited []struct {
		Email, By string
		Expires   int64
		Expired   bool
	}
	Tabs []struct {
		Key, Label, Href string
		Count            int
		On               bool
	}
	Tab                                                            string
	Pages                                                          struct{ Shown, Previous, Next string }
	Manage, RoleAction, Back, Unreadable, SpendUnreadable, Refused string
	Admins                                                         int
	Tokens, Budgets                                                bool
}

type personJSON struct {
	Email, Name, Role string
	LastSignIn        int64
	Budget            struct {
		Set, Passed bool
		Limit       float64
	}
	Spent *struct {
		USD               float64 `json:"usd"`
		Calls, Unmeasured int64
	}
}

// people is the People page's data at the query given.
func people(t *testing.T, here *site, query string) peopleJSON {
	t.Helper()
	var page peopleJSON
	here.data(t, "/admin/people"+query, &page)
	return page
}

// personIn is one person's row of the People page, by their address.
func personIn(t *testing.T, page peopleJSON, email string) personJSON {
	t.Helper()
	for _, each := range page.People {
		if each.Email == email {
			return each
		}
	}
	t.Fatalf("People has no row for %s", email)
	return personJSON{}
}

// spentOf is what a person's row says they spent, or nothing when it says nothing.
func (p personJSON) spentOf() float64 {
	if p.Spent == nil {
		return 0
	}
	return p.Spent.USD
}

// dataReader is a signed-in site, or a test fixture that holds one.
type dataReader interface {
	data(t *testing.T, path string, into any)
}

// usageJSON is a Usage page's data: the console's, or a person's own.
type usageJSON struct {
	Base string
	Tabs struct{ Summary, Tasks, Current string }
	Mine *struct {
		Set, Passed  bool
		Limit, Spent float64
	}
	Usage *usageViewJSON
}

func usagePageOf(t *testing.T, here dataReader, query string) usageJSON {
	t.Helper()
	return usageAt(t, here, "/admin/usage"+query)
}

// usageAt is the data of the usage page at path.
func usageAt(t *testing.T, here dataReader, path string) usageJSON {
	t.Helper()
	var page usageJSON
	here.data(t, path, &page)
	return page
}

// sessionJSON is a session page's data.
type sessionJSON struct {
	Base    string
	Session struct {
		ID, Person, Agent, Engine, Search string
		Next, Newest                      string
		Calls                             []struct {
			At, AtFull, Task, Record, Model, Provider, Key string
			In, Out, Cache, Cost, Refused, Translated      string
			Status                                         int
			Unpriced                                       bool
		}
	}
}

// sessionAt is the data of the session page at path.
func sessionAt(t *testing.T, here dataReader, path string) sessionJSON {
	t.Helper()
	var page sessionJSON
	here.data(t, path, &page)
	return page
}

// sessionsOf is the ids of the sessions a page of the usage lists.
func (u usageJSON) sessionsOf() []string {
	if u.Usage == nil {
		return nil
	}
	return u.Usage.sessionIDs()
}

// usageViewJSON is the usage index as a Usage page shows it.
type usageViewJSON struct {
	Values   map[string][]string
	Everyone bool
	Facets   usage.Facets
	Building usage.Status
	Figures  struct {
		Spent, SpentCaption, In, Out, Cache, Unpriced string
		Calls, Refused                                int64
	}
	Rows []struct {
		ID, Href, Asked, Agent, Engine, Owner, Providers, Models, Cost, Unpriced string
		Turns                                                                    int
	}
	Columns []struct {
		Label, Href, Sorted string
		Num                 bool
	}
	States []struct {
		Label, Href, Count string
		Current            bool
	}
	ShowWho, Filtered           bool
	Clear, Export, Newest, Next string
}

func (v *usageViewJSON) sessionIDs() []string {
	var ids []string
	for _, row := range v.Rows {
		ids = append(ids, row.ID)
	}
	return ids
}
