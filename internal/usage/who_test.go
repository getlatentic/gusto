package usage_test

import (
	"testing"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/usage"
)

func TestWhoOfATaskInNoConversationIsTheTaskItself(t *testing.T) {
	who := usage.WhoOfTask(jobs.Record{ID: "t1", Asked: "ada@acme.com", Team: "platform", Repo: "r", Harness: "opencode"},
		func(h string) string { return "Engine " + h }, func(string, string) (string, bool) { t.Fatal("no agent to ask"); return "", false })
	want := usage.Who{Session: "t1", Person: "ada@acme.com", Team: "platform", Project: "r", Engine: "Engine opencode"}
	if who != want {
		t.Fatalf("%+v", who)
	}
}

func TestWhoOfATurnIsTheInstanceAndThreadAndTheAgentsName(t *testing.T) {
	who := usage.WhoOfTask(jobs.Record{ID: "t2", Asked: "ada@acme.com", Instance: "i1", Thread: "th"},
		func(h string) string { return h }, func(id, _ string) (string, bool) { return "Reviewer of " + id, true })
	if who.Session != "i1:th" || who.Agent != "Reviewer of i1" {
		t.Fatalf("%+v", who)
	}
	if who := usage.WhoOfTask(jobs.Record{Instance: "gone"}, func(h string) string { return h }, func(string, string) (string, bool) { return "", false }); who.Agent != "" || who.Session != "gone" {
		t.Fatalf("%+v", who)
	}
}

func TestAnAgentIsNamedByItsTitleThenTemplateThenEngine(t *testing.T) {
	for _, c := range []struct{ title, template, engine, want string }{
		{"Mine", "T", "E", "Mine"}, {"", "T", "E", "T"}, {"", "", "E", "E"}, {"", "", "", "Agent"},
	} {
		if got := usage.AgentName(c.title, c.template, c.engine); got != c.want {
			t.Errorf("%+v gave %q", c, got)
		}
	}
}
