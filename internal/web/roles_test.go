package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestARoleComesFromTheGroupsTheProviderNames(t *testing.T) {
	company := Access{RunnerGroups: []string{"engineering", "platform"}, AdminGroups: []string{"haas-admins"}}
	for _, c := range []struct {
		name   string
		groups []string
		role   Role
		in     bool
	}{
		{"a runner group", []string{"sales", "engineering"}, Runner, true},
		{"the other runner group", []string{"platform"}, Runner, true},
		{"an admin group", []string{"haas-admins"}, Admin, true},
		{"both, where the admin group outranks", []string{"engineering", "haas-admins"}, Admin, true},
		{"a group nothing maps", []string{"sales"}, "", false},
		{"no groups at all", nil, "", false},
		{"a group that only looks like one", []string{"Engineering", "engineering-interns"}, "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			role, in := company.roleOf(Person{Email: "somebody@company.example", Groups: c.groups})
			if role != c.role || in != c.in {
				t.Errorf("got %q admitted=%v, want %q admitted=%v", role, in, c.role, c.in)
			}
		})
	}
}

// A provider with no groups to name is gated by the allowed list alone, and
// everybody it lets in runs work.
func TestWithNoGroupsNamedTheAllowedListIsTheGate(t *testing.T) {
	only := Access{Allowed: []string{"@company.example"}}
	if role, in := only.roleOf(Person{Email: "a@company.example"}); !in || role != Runner {
		t.Errorf("an allowed address got %q admitted=%v", role, in)
	}
	if _, in := only.roleOf(Person{Email: "a@elsewhere.example", Groups: []string{"admins"}}); in {
		t.Error("an address the list does not name was let in")
	}
}

// The allowed list is a restriction on top of the groups, not a second way in.
func TestTheAllowedListStillRestrictsWhatTheGroupsGrant(t *testing.T) {
	both := Access{Allowed: []string{"@company.example"}, RunnerGroups: []string{"engineering"}}
	if _, in := both.roleOf(Person{Email: "a@contractor.example", Groups: []string{"engineering"}}); in {
		t.Error("somebody in the group but outside the allowed domain was let in")
	}
	if role, in := both.roleOf(Person{Email: "a@company.example", Groups: []string{"engineering"}}); !in || role != Runner {
		t.Errorf("somebody in both got %q admitted=%v", role, in)
	}
}

func TestGroupsAreReadWhetherTheProviderSendsAListOrOneString(t *testing.T) {
	for raw, want := range map[string]string{
		`["a","b"]`: "a,b",
		`"only"`:    "only",
		`[]`:        "",
		`null`:      "",
		`7`:         "",
		``:          "",
	} {
		if got := strings.Join(groupsIn(json.RawMessage(raw)), ","); got != want {
			t.Errorf("%q read as %q, want %q", raw, got, want)
		}
	}
}

// A provider that lets everybody with an account through, with nothing here to
// tell them apart, would let anybody spend the credential.
func TestAProviderWithNothingToTellPeopleApartIsRefusedAtStartup(t *testing.T) {
	_, err := NewIdentity(context.Background(), "https://issuer.invalid", "id", "secret", "https://haas.example/auth/callback", Access{})
	if !errors.Is(err, errUngated) {
		t.Fatalf("got %v, want the refusal to start ungated", err)
	}
	for name, gated := range map[string]Access{
		"an allowed list": {Allowed: []string{"@company.example"}},
		"a runner group":  {RunnerGroups: []string{"engineering"}},
		"an admin group":  {AdminGroups: []string{"haas-admins"}},
	} {
		_, err := NewIdentity(context.Background(), "https://issuer.invalid", "id", "secret", "https://haas.example/auth/callback", gated)
		if errors.Is(err, errUngated) {
			t.Errorf("%s was refused as ungated", name)
		}
	}
}

// HAAS_ADMINS is how an operator keeps a way in whatever the groups say.
func TestAnAddressNamedAnAdminIsAdmittedWhateverTheirGroups(t *testing.T) {
	site := open(t)
	site.server.identity = &Identity{access: Access{RunnerGroups: []string{"engineering"}}}
	site.server.UseAdmins([]string{"Owner@Company.example"})

	if role, in := site.server.roleFor(Person{Email: "owner@company.example"}); !in || role != Admin {
		t.Errorf("the named admin got %q admitted=%v", role, in)
	}
	if _, in := site.server.roleFor(Person{Email: "other@company.example", Groups: []string{"sales"}}); in {
		t.Error("somebody in no mapped group was let in")
	}
}

// Changing what agents are given changes what every later run may do, so a
// person who only runs work cannot.
func TestARunnerCannotChangeWhatAgentsAreGiven(t *testing.T) {
	site := open(t)
	site.server.UseAdmins([]string{"admin@example.com"})
	admin, runner := site.as(t, "admin@example.com"), site.as(t, alice)

	if made := admin.ask(t, "POST", "/setups", url.Values{"name": {"reviewer"}}); made.Code != http.StatusSeeOther {
		t.Fatalf("an admin could not make a setup: %d %s", made.Code, made.Body.String())
	}
	for _, change := range []struct {
		path string
		form url.Values
	}{
		{"/setups", url.Values{"name": {"another"}}},
		{"/setups/reviewer/skills", url.Values{"skill": {"review"}, "description": {"when"}, "body": {"do"}}},
		{"/setups/reviewer/skills/review/drop", nil},
	} {
		if got := runner.ask(t, "POST", change.path, change.form); got.Code != http.StatusForbidden {
			t.Errorf("a runner POSTing %s got %d", change.path, got.Code)
		}
	}
	if _, found := site.server.setups.Get("another"); found {
		t.Error("a runner made a setup")
	}
	if got := admin.ask(t, "POST", "/setups/reviewer/skills", url.Values{"skill": {"review"}, "description": {"when"}, "body": {"do"}}); got.Code != http.StatusSeeOther {
		t.Errorf("an admin adding a skill got %d", got.Code)
	}
}

// A runner may read what agents are given, and is not offered what they cannot
// use.
func TestARunnerReadsSetupsAndIsNotOfferedTheControlsToChangeThem(t *testing.T) {
	site := open(t)
	site.server.UseAdmins([]string{"admin@example.com"})
	admin, runner := site.as(t, "admin@example.com"), site.as(t, alice)
	admin.ask(t, "POST", "/setups", url.Values{"name": {"reviewer"}})
	admin.ask(t, "POST", "/setups/reviewer/skills", url.Values{"skill": {"review"}, "description": {"when"}, "body": {"do"}})

	for path, controls := range map[string][]string{
		"/setups":          {"New skill set"},
		"/setups/reviewer": {"Add a skill", "/skills/review/drop"},
	} {
		if got := runner.ask(t, "GET", path, nil); got.Code != http.StatusOK {
			t.Fatalf("a runner reading %s got %d", path, got.Code)
		} else {
			for _, control := range controls {
				if strings.Contains(got.Body.String(), control) {
					t.Errorf("%s offers a runner %q", path, control)
				}
			}
		}
		for _, control := range controls {
			if !strings.Contains(admin.ask(t, "GET", path, nil).Body.String(), control) {
				t.Errorf("%s does not offer an admin %q", path, control)
			}
		}
	}
}
