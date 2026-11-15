package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/crew"
)

func TestAnAgentIsMadeInMembersAndStartsPrivate(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	mine, his := site.as(t, alice), site.as(t, bob)
	made := mine.ask(t, "POST", "/members/agents", url.Values{"name": {"Atlas"}, "specialty": {"Research"}, "harness": {"other"}, "model": {"platform/test-model"}})
	where := made.Header().Get("Location")
	if made.Code != http.StatusSeeOther || !strings.HasPrefix(where, "/members/agents/") {
		t.Fatalf("making an agent gave %d to %s", made.Code, where)
	}
	member, _ := site.server.crew.Get(strings.TrimPrefix(where, "/members/agents/"))
	if member.Name != "Atlas" || member.Harness != "other" || member.Owner != alice || len(member.Access) != 0 {
		t.Fatalf("made %+v", member)
	}
	if !his.members(t, "/members").listsAgent("Atlas") {
		t.Error("the members list does not show the workspace's agent to everybody")
	}
	if got := his.ask(t, "POST", where, url.Values{"name": {"Mine now"}}); got.Code != http.StatusForbidden {
		t.Errorf("somebody else changed the agent: %d", got.Code)
	}
	if got := his.ask(t, "POST", where+"/message", nil); got.Code != http.StatusForbidden {
		t.Errorf("somebody without access messaged the agent: %d", got.Code)
	}
	for _, refused := range []url.Values{
		{"name": {"Atlas"}, "harness": {"other"}},
		{"name": {"Nobody"}, "harness": {"not-here"}},
		{"name": {"Pricey"}, "harness": {"other"}, "model": {"acme/not-offered"}},
	} {
		if got := mine.ask(t, "POST", "/members/agents", refused); !strings.Contains(got.Header().Get("Location"), "refused") {
			t.Errorf("%v was not refused", refused)
		}
	}
}

func TestSharingAnAgentLetsAnotherPersonMessageIt(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine, his := site.as(t, alice), site.as(t, bob)
	where := "/members/agents/" + member.ID
	mine.ask(t, "POST", where+"/access", url.Values{"who": {"Bob@Example.com"}})
	if shared, _ := site.server.crew.Get(member.ID); !shared.MayUse(bob) {
		t.Fatalf("bob was not given access: %+v", shared.Access)
	}
	opened := his.ask(t, "POST", where+"/message", nil)
	if opened.Code != http.StatusSeeOther {
		t.Fatalf("messaging gave %d", opened.Code)
	}
	again := his.ask(t, "POST", where+"/message", nil)
	if again.Header().Get("Location") != opened.Header().Get("Location") {
		t.Errorf("messaging twice opened %s and %s", opened.Header().Get("Location"), again.Header().Get("Location"))
	}
	direct, found := site.server.instances.DirectRoom(bob, member.ID)
	if !found || !direct.Direct || direct.RoleOf(alice) != "" {
		t.Fatalf("the direct room is %+v", direct)
	}
	if direct := shellOf(t, his).Rail.Direct; !slices.ContainsFunc(direct, func(room railChat) bool { return room.Name == "Atlas" }) {
		t.Error("the side list does not carry the direct room under Direct messages")
	}
	mine.ask(t, "POST", where+"/access", url.Values{"who": {bob}, "action": {"remove"}})
	if got := his.ask(t, "POST", where+"/message", nil); got.Code != http.StatusForbidden {
		t.Errorf("bob still messaged the agent after losing access: %d", got.Code)
	}
}

func TestRemovingAnAgentTakesItOutOfItsRooms(t *testing.T) {
	work := &roomWork{}
	here, id, _ := aRoom(t, work, [2]string{"Scout", "fixture"})
	held, _ := here.server.instances.Get(id)
	member := held.Agents[0].Member
	if got := here.as(t, bob).ask(t, "POST", "/members/agents/"+member+"/remove", nil); got.Code != http.StatusForbidden {
		t.Errorf("somebody else removed the agent: %d", got.Code)
	}
	here.as(t, alice).ask(t, "POST", "/members/agents/"+member+"/remove", nil)
	if _, found := here.server.crew.Get(member); found {
		t.Error("the agent is still kept")
	}
	if held, _ := here.server.instances.Get(id); len(held.Agents) != 0 {
		t.Errorf("the room still holds %+v", held.Agents)
	}
}

func TestAnAgentsPageShowsItsProfileAccessAndSkills(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine := site.as(t, alice)
	where := "/members/agents/" + member.ID
	mine.ask(t, "POST", where, url.Values{"tab": {"profile"}, "persona": {"You are careful."}, "specialty": {"Research"}})
	page := mine.members(t, where+"?tab=profile")
	if shown := page.Agent; shown == nil || shown.Persona != "You are careful." || shown.Specialty != "Research" || !shown.Mine || !shown.Usable {
		t.Errorf("the agent's page shows %+v", shown)
	}
	if shown := page.Agent; shown.OwnerName == "" || shown.Created == 0 || len(shown.Access) != 0 || shown.Engine == "" {
		t.Errorf("the agent's page does not say who made it, when, and who else may work with it: %+v", shown)
	}
	if page.Skills == nil || page.Skills.Given || len(page.Skills.Choices) == 0 {
		t.Errorf("the skills tab does not offer skills to an agent given none: %+v", page.Skills)
	}
	if theirs := site.as(t, bob).members(t, where).Agent; theirs == nil || theirs.Mine || theirs.Usable {
		t.Errorf("somebody else is offered the agent to edit or use: %+v", theirs)
	}
	if got := mine.ask(t, "GET", "/members/agents/000000000000", nil); got.Code != http.StatusNotFound {
		t.Errorf("a missing agent answered %d", got.Code)
	}
	person := mine.members(t, "/members/people/"+url.PathEscape(alice))
	if len(person.Made) != 1 || person.Made[0].Name != "Atlas" || person.Made[0].Where != where {
		t.Errorf("a person's page does not list the agents they made: %+v", person.Made)
	}
}

func TestADirectRoomStillOpensAfterItsAgentIsRemoved(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine := site.as(t, alice)
	opened := mine.ask(t, "POST", "/members/agents/"+member.ID+"/message", nil).Header().Get("Location")
	if page := mine.conversation(t, opened); !page.Instance.Direct || page.DirectAgent == nil || page.DirectAgent.Name != "Atlas" || page.Hint != "Message Atlas" {
		t.Fatalf("the direct room does not say whose it is: %+v %+v", page.Instance, page.DirectAgent)
	}
	mine.ask(t, "POST", "/members/agents/"+member.ID+"/remove", nil)
	if got := mine.ask(t, "GET", opened, nil); got.Code != http.StatusOK {
		t.Errorf("the direct room answered %d once its agent was removed", got.Code)
	}
	mine.conversation(t, opened)
}

func TestTheNewAgentDialogShowsEveryAgentTypeAndWhyOneCannotRun(t *testing.T) {
	kinds := append(twoHarnesses(), catalog.Entry{Name: "claude", Title: "Claude Code", Program: "claude", Ready: false, Why: "npm install -g @anthropic-ai/claude-code"})
	site := openOffering(t, kinds)
	mine := site.as(t, alice)
	page := mine.members(t, "/members")
	if claude := page.kind(t, "claude"); claude.Title != "Claude Code" || claude.Ready || claude.Installable || claude.Why != "npm install -g @anthropic-ai/claude-code" {
		t.Errorf("the new agent dialog does not say why Claude Code cannot run: %+v", claude)
	}
	if !page.kind(t, "other").Ready || page.NewAgent.FirstReady == "" || page.NewAgent.FirstReady == "claude" {
		t.Errorf("the new agent dialog does not start on an agent type that can run: %+v", page.NewAgent)
	}
	if got := mine.ask(t, "POST", "/members/agents", url.Values{"name": {"Atlas"}, "harness": {"claude"}}); !strings.Contains(got.Header().Get("Location"), "refused") {
		t.Error("an agent type the server cannot run was taken")
	}
}

// installs is an installer that knows one agent, and keeps what it was asked to install.
type installs struct {
	asked []string
	fails error
}

func (i *installs) Installable(name string) bool { return name == "claude" }

func (i *installs) Install(_ context.Context, name string) (string, error) {
	i.asked = append(i.asked, name)
	return name + "@1.0.0", i.fails
}

func TestGustoOnAPersonsOwnComputerInstallsAnAgentItLacks(t *testing.T) {
	kinds := append(twoHarnesses(), catalog.Entry{Name: "claude", Title: "Claude Code", Program: "claude-agent-acp", Why: "npm install -g @agentclientprotocol/claude-agent-acp@1.0.0"})
	site := openOffering(t, kinds)
	site.signIn(t)
	installer := &installs{}
	site.server.UseInstaller(installer)
	if site.members(t, "/members").kind(t, "claude").Installable {
		t.Error("a shared server offers to install an agent on itself")
	}
	if got := site.ask(t, "POST", "/agents/claude/install", nil); !strings.Contains(got.Header().Get("Location"), "refused") || len(installer.asked) != 0 {
		t.Error("a shared server installed an agent")
	}

	site.server.UseLocalSignIn("a-secret", "ada")
	if claude := site.members(t, "/members").kind(t, "claude"); !claude.Installable || claude.Ready {
		t.Fatalf("a person's own gusto does not offer to install the agent it lacks: %+v", claude)
	}
	got := site.ask(t, "POST", "/agents/claude/install", url.Values{"back": {"/members?new=1"}})
	if got.Header().Get("Location") != "/members?new=1" || len(installer.asked) != 1 {
		t.Errorf("installing went to %s, asked %v", got.Header().Get("Location"), installer.asked)
	}
	installer.fails = errors.New("npm could not fetch it")
	if got := site.ask(t, "POST", "/agents/claude/install", url.Values{"back": {"//elsewhere.example"}}); !strings.HasPrefix(got.Header().Get("Location"), "/members?new=1&refused=npm") {
		t.Errorf("a failed install went to %s", got.Header().Get("Location"))
	}
	if got := site.ask(t, "POST", "/agents/opencode/install", nil); !strings.Contains(got.Header().Get("Location"), "refused") || len(installer.asked) != 2 {
		t.Error("an agent gusto cannot install was installed")
	}
}

// Editing an agent's profile changes only the profile: people given access, or app choices made, while the edit was on its way stay.
func TestEditingAnAgentsProfileKeepsAccessGivenMeanwhile(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine := site.as(t, alice)
	const given = 40
	done := make(chan error, 1)
	go func() {
		for n := range given {
			if _, err := site.server.crew.Change(member.ID, func(held *crew.Agent) error {
				held.Access = append(held.Access, fmt.Sprintf("p%d@acme.com", n))
				return nil
			}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for n := range given {
		mine.ask(t, "POST", "/members/agents/"+member.ID, url.Values{"tab": {"profile"}, "persona": {fmt.Sprintf("Version %d.", n)}})
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	held, _ := site.server.crew.Get(member.ID)
	if len(held.Access) != given {
		t.Errorf("%d of %d people given access meanwhile kept it", len(held.Access), given)
	}
	if held.Persona != fmt.Sprintf("Version %d.", given-1) {
		t.Errorf("the profile edit was lost: %q", held.Persona)
	}
}
