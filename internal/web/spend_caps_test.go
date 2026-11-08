package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
)

// One person's own budget of $0 allows them nothing; it is not taken for no budget.
func TestOnePersonsBudgetOfNothingAllowsNothing(t *testing.T) {
	here, log, _ := budgeted(t)
	here.ask(t, "POST", "/admin/member-budget", url.Values{"who": {bob}, "usd": {"0"}})
	settings, err := here.server.orgSettings.Read()
	if err != nil {
		t.Fatal(err)
	}
	if usd, set := settings.MemberBudget(bob); !set || usd != 0 {
		t.Errorf("bob's $0 budget reads as %v, set %v", usd, set)
	}
	if row := rowOf(t, budgets(t, here), bob); !strings.Contains(row, `value="0.00"`) {
		t.Errorf("bob's row does not show his $0 budget:\n%s", row)
	}
	if set := ofKind(written(t, log), changelog.MemberBudgetSet); len(set) != 1 || set[0].What["now"] != "$0.00" {
		t.Errorf("the record has %+v", set)
	}
}

// The organization's budget is passed once what was spent comes to it, as every cap in the proxy is.
func TestTheOrganizationsBudgetIsReachedAtItsLimit(t *testing.T) {
	here, _, _ := budgeted(t)
	setBudget(t, here, "50")
	if state := here.server.budgetOf(50); !state.Passed {
		t.Errorf("$50 spent of a $50 budget is not passed: %+v", state)
	}
	if state := here.server.budgetOf(49.99); state.Passed {
		t.Errorf("$49.99 spent of a $50 budget is passed: %+v", state)
	}
}

// An agent's owner, or the room's admin, changes what it may spend in the room; nobody else does.
func TestAnAgentsOwnerOrTheRoomsAdminChangesItsAllowance(t *testing.T) {
	site, id, _ := aRoomWithBob(t, &roomWork{})
	atlas := bobsAgentIn(t, site, id)
	allowance := func(who, agent, typed string) *http.Response {
		return site.as(t, who).ask(t, "POST", "/rooms/"+id+"/agents/"+agent+"/allowance", url.Values{"allowance": {typed}}).Result()
	}
	if got := allowance(bob, atlas.ID, "5"); got.StatusCode != http.StatusSeeOther {
		t.Fatalf("bob raising his agent's allowance: %d", got.StatusCode)
	}
	held, _ := site.server.instances.Get(id)
	if agent, _ := held.AgentByID(atlas.ID); agent.AllowanceUSD == nil || *agent.AllowanceUSD != 5 {
		t.Fatalf("the allowance is %v", agent.AllowanceUSD)
	}
	allowance(alice, atlas.ID, "")
	held, _ = site.server.instances.Get(id)
	if agent, _ := held.AgentByID(atlas.ID); agent.AllowanceUSD != nil {
		t.Errorf("the room's admin cleared the allowance, and it is %v", *agent.AllowanceUSD)
	}
	scout, _ := held.AgentCalled("Scout")
	if got := allowance(bob, scout.ID, "100"); got.StatusCode != http.StatusForbidden {
		t.Errorf("bob changed the allowance of an agent he does not own: %d", got.StatusCode)
	}
}
