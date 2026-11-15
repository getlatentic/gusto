package web

import "testing"

// chatJSON is the chat home's data, as the React page reads it from /app/api/chat.
type chatJSON struct {
	Ready          bool
	Harness, Model string
	Agents         []option
	Models         map[string][]option
	Refused        string
	Start          startFolder
	Roles          []roleChoice
	Role           string
	NeedsModel     bool
	Presets        []modelPreset
	Missing        []missingAgent
	HandInstall    bool
	FoundKeys      []string
	Lacking        []string
	Wall           *struct {
		Short, Words string
		Missing      []string
		Unconfirmed  bool
	}
}

func chatOf(t *testing.T, here *site, query string) chatJSON {
	t.Helper()
	var chat chatJSON
	here.data(t, "/chat"+query, &chat)
	return chat
}

// offers says whether a picker's options hold the value.
func offers(options []option, value string) bool {
	for _, each := range options {
		if each.Value == value {
			return true
		}
	}
	return false
}

// roleOf is the role menu's entry with the value, and whether there is one.
func (c chatJSON) roleOf(value string) (roleChoice, bool) {
	for _, each := range c.Roles {
		if each.Value == value && each.Kind != "heading" && each.Kind != "separator" {
			return each, true
		}
	}
	return roleChoice{}, false
}
