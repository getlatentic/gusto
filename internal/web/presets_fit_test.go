package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/industry"
)

func TestAnAgentThatCannotTakeAPresetIsToldSoInTheWordsOfTheChoice(t *testing.T) {
	got := unfitSaid("OpenCode", "Clinical documentation", "it takes no skills")
	want := "OpenCode cannot run Clinical documentation, which needs skills OpenCode does not take. Choose another agent."
	if got != want {
		t.Errorf("%q", got)
	}
	if tools := unfitSaid("Pi", "Reviewer", "it takes no MCP servers"); !strings.Contains(tools, "needs tool servers") {
		t.Errorf("%q", tools)
	}
}

func TestEachBuiltInPresetCarriesTheAgentsThatCanTakeIt(t *testing.T) {
	site := open(t)
	site.signIn(t)
	var choices []roleChoice
	if err := json.Unmarshal([]byte(site.server.roleChoices(httptest.NewRequest("GET", "/chat", nil))), &choices); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, choice := range choices {
		name, built := strings.CutPrefix(choice.Value, "r:")
		if !built {
			continue
		}
		seen++
		template, _ := industry.Get(name)
		if len(choice.Fit) != len(site.server.runnersOf(template)) {
			t.Errorf("%s lists %d agents and %d can take it", name, len(choice.Fit), len(site.server.runnersOf(template)))
		}
		if len(choice.Fit) == 0 && choice.Detail != "No agent here can take this yet" {
			t.Errorf("%s fits no agent and says %q", name, choice.Detail)
		}
	}
	if seen == 0 {
		t.Error("no built-in presets were offered")
	}
}
