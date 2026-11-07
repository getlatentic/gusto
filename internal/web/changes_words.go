package web

import (
	"fmt"

	"github.com/getlatentic/gusto/internal/changelog"
)

// saidOf says an event in words a person reads.
func saidOf(e changelog.Event) string {
	if words, found := eventWords[e.Kind]; found {
		return words(e, e.What)
	}
	return e.Kind
}

// eventWords is what each kind of event says happened, from the event and what it carries.
var eventWords = map[string]func(e changelog.Event, w map[string]string) string{
	changelog.SignedIn: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("signed in as %s, with %s", or(e.Role, "no role"), or(w["how"], "an unknown method"))
	},
	changelog.Refused: func(e changelog.Event, w map[string]string) string {
		said := "was refused with " + or(w["how"], "an unknown method") + ": " + or(w["why"], "no reason given")
		if w["suppressed"] != "" {
			said += " (" + w["suppressed"] + " more from here were not written)"
		}
		return said
	},
	changelog.SignedOut: func(e changelog.Event, w map[string]string) string { return "signed out" },
	changelog.RoleChanged: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("role changed from %s to %s", or(w["was"], "none"), or(w["now"], "none"))
	},
	changelog.SetupSaved: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("%s skill set %s, with tools: %s", w["action"], w["setup"], or(w["servers"], "none"))
	},
	changelog.SkillWritten: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("wrote the skill %s in %s", w["skill"], w["setup"])
	},
	changelog.SkillRestored: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("put back an earlier version of the skill %s in %s (%s, was %s)", w["skill"], w["setup"], or(w["now"], "none"), or(w["was"], "none"))
	},
	changelog.SetupEnabled:  saidSetupTurned,
	changelog.SetupDisabled: saidSetupTurned,
	changelog.SkillDropped: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("removed the skill %s from %s", w["skill"], w["setup"])
	},
	changelog.TokenCreated: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("made the API token %s, which may %s", w["name"], or(w["scopes"], "do nothing"))
	},
	changelog.TokenRevoked: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("revoked the API token %s", w["name"])
	},
	changelog.HoldSet: func(e changelog.Event, w map[string]string) string {
		return "kept " + heldThing(w) + " from being removed" + becauseOf(w["why"])
	},
	changelog.HoldReleased: func(e changelog.Event, w map[string]string) string {
		return "let " + heldThing(w) + " be removed on schedule again"
	},
	changelog.Swept: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("retention removed %s working folders and %s records, freeing %s bytes%s",
			or(w["workspaces"], "0"), or(w["records"], "0"), or(w["freed"], "0"), recordsOf(w["jobs"]))
	},
	changelog.SandboxConfirmed: func(e changelog.Event, w map[string]string) string {
		return "confirmed that agents run with no sandbox between them and their files"
	},
	changelog.ChatDefaultSet: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("changed what a new chat runs on from %s to %s", or(w["was"], "the first that can run"), or(w["now"], "the first that can run"))
	},
	changelog.RunTokensSet: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("changed the model tokens of runs in sandboxes from %s to %s", w["was"], w["now"])
	},
	changelog.BudgetSet: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("changed the monthly budget from %s to %s", or(w["was"], "none"), or(w["now"], "none"))
	},
	changelog.ConnectorEnabled:  saidConnectorTurned,
	changelog.ConnectorDisabled: saidConnectorTurned,
	changelog.ConnectorConnected: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("signed in to %s with their own account, for their agents", w["connector"])
	},
	changelog.ConnectorsLetInRoom: func(e changelog.Event, w map[string]string) string {
		if w["now"] == "on" {
			return fmt.Sprintf("let the agents in %s use their connected apps, for what they ask", w["room"])
		}
		return fmt.Sprintf("stopped the agents in %s using their connected apps", w["room"])
	},
	changelog.ConnectorsLetAgent: func(e changelog.Event, w map[string]string) string { return agentAppWords(w) },
	changelog.ConnectorForAgents: func(e changelog.Event, w map[string]string) string {
		if w["now"] == "on" {
			return fmt.Sprintf("let their agents use %s unless they say otherwise for one", w["connector"])
		}
		return fmt.Sprintf("kept their agents from %s unless they allow one", w["connector"])
	},
	changelog.ConnectorDisconnected: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("took their %s sign-in away from their agents", w["connector"])
	},
	changelog.MemberBudgetSet: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("changed %s monthly budget from %s to %s", or(w["whose"], "every member's"), or(w["was"], "none"), or(w["now"], "none"))
	},
	changelog.ProviderAdded: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("added the model provider %s at %s, with its key %s", w["provider"], w["address"], or(w["key"], "kept"))
	},
	changelog.ProviderRetired: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("retired the model provider %s", w["provider"])
	},
	changelog.AddressAdded: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("gave the model provider %s the %s address %s", w["provider"], w["kind"], w["address"])
	},
	changelog.AddressRemoved: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("took the address %s away from the model provider %s", w["address"], w["provider"])
	},
	changelog.KeyReplaced: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("gave the model provider %s another key", w["provider"])
	},
	changelog.ModelAdded: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("added the model %s, priced %s", w["model"], or(w["price"], "none"))
	},
	changelog.ProviderEnabled:  saidProviderTurned,
	changelog.ProviderDisabled: saidProviderTurned,
	changelog.ModelEnabled:     saidModelTurned,
	changelog.ModelDisabled:    saidModelTurned,
	changelog.RegistryRestored: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("put the models back as they were at version %s, which undid: %s", w["version"], or(w["undid"], "nothing"))
	},
	changelog.IntegrationImported: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("imported %s %s (%s)", w["integration"], or(w["label"], w["version"]), or(w["source"], "upload"))
	},
	changelog.IntegrationEnabled: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("turned on %s, version %s, in place of %s%s", w["integration"], w["to"], or(w["from"], "none"), becauseOf(w["changes"]))
	},
	changelog.IntegrationDisabled: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("turned off %s, which was version %s", w["integration"], or(w["from"], "none"))
	},
	changelog.IntegrationRestored: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("put %s back to version %s, from %s", w["integration"], w["to"], or(w["from"], "none"))
	},
	changelog.IntegrationPrograms: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("changed which version of %s may run its programs, from %s to %s", w["integration"], or(w["was"], "none"), or(w["allowed"], "none"))
	},
	changelog.IntegrationScoped: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("changed who %s is on for, from %s to %s", w["integration"], or(w["was"], "everybody"), or(w["now"], "everybody"))
	},
	changelog.ModelRetired: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("retired the model %s", w["model"])
	},
	changelog.PriceSet: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("priced the model %s at %s", w["model"], or(w["price"], "none"))
	},
	changelog.PolicySet: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("set what model providers may be: private addresses %s, keys from %s", or(w["private"], "no"), or(w["keys"], "a typed key only"))
	},
	changelog.AccessSet: func(e changelog.Event, w map[string]string) string {
		if w["models"] == "" {
			return fmt.Sprintf("let %s use every model again", w["email"])
		}
		return fmt.Sprintf("limited %s to the models %s", w["email"], w["models"])
	},
	changelog.MemberInvited: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("invited %s to join as a %s", w["email"], or(w["role"], "runner"))
	},
	changelog.MemberRole: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("changed the role of %s from %s to %s", w["email"], or(w["was"], "none"), or(w["now"], "none"))
	},
	changelog.MemberRemoved: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("removed %s, who was %s", w["email"], or(w["role"], "a member"))
	},
	changelog.WorkspaceMade: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("made the workspace %s", w["name"])
	},
	changelog.WorkspaceRenamed: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("renamed the workspace %s to %s", w["workspace"], w["name"])
	},
	changelog.WorkspaceAdded: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("added %s to the workspace %s", w["email"], w["workspace"])
	},
	changelog.WorkspaceLeft: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("removed %s from the workspace %s", w["email"], w["workspace"])
	},
	changelog.ServerStarted: func(e changelog.Event, w map[string]string) string { return "the server started" },
	changelog.SettingsChanged: func(e changelog.Event, w map[string]string) string {
		return fmt.Sprintf("the setting %s changed from %q to %q", w["setting"], w["was"], w["now"])
	},
}

func saidSetupTurned(_ changelog.Event, w map[string]string) string {
	return fmt.Sprintf("turned the skill set %s %s", w["setup"], w["now"])
}

func saidConnectorTurned(_ changelog.Event, w map[string]string) string {
	return fmt.Sprintf("turned the connector %s %s", w["connector"], w["now"])
}

func saidProviderTurned(_ changelog.Event, w map[string]string) string {
	return fmt.Sprintf("turned the model provider %s %s", w["provider"], w["now"])
}

func saidModelTurned(_ changelog.Event, w map[string]string) string {
	return fmt.Sprintf("turned the model %s %s", w["model"], w["now"])
}
