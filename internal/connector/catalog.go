// Package connector signs a person in to an app's MCP server with their own account, and keeps the tokens that sign-in gives so
// the gateway can call the server as them. A connector is a remote MCP server that asks for OAuth; the person signs in at the app,
// and the agent never sees what the app gave back.
package connector

import (
	"fmt"
	"slices"

	"github.com/getlatentic/gusto/pkg/job"
)

// Entry is a connector an organization may turn on: an app's MCP server, and whether the operator must register a client with
// the app first, which an app that registers clients itself does not need.
type Entry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// What is the one line a person reads before they sign in.
	What string `json:"what"`
	URL  string `json:"url"`
	// NeedsClient is an app that has no way for a client to register itself, so the operator registers one and gives its id.
	NeedsClient bool `json:"needsClient,omitempty"`
	// Scopes is what a sign-in asks for, in place of everything the app's server lists, for an app that lists more than an agent
	// needs.
	Scopes []string `json:"scopes,omitempty"`
}

// Catalog is the connectors gusto knows. Each server was asked on 31 Oct 2026 and answered as MCP's authorization asks: a 401
// naming its resource metadata. The ones that need no client registered either take a client ID metadata document or register
// clients themselves.
var Catalog = []Entry{
	{ID: "linear", Name: "Linear", What: "Issues, projects and cycles in your Linear workspace", URL: "https://mcp.linear.app/mcp"},
	{ID: "notion", Name: "Notion", What: "Pages and databases in your Notion workspace", URL: "https://mcp.notion.com/mcp"},
	{ID: "figma", Name: "Figma", What: "Files, frames and comments in your Figma", URL: "https://mcp.figma.com/mcp"},
	{ID: "canva", Name: "Canva", What: "Designs in your Canva", URL: "https://mcp.canva.com/mcp"},
	{ID: "stripe", Name: "Stripe", What: "Customers, payments and products in your Stripe account", URL: "https://mcp.stripe.com"},
	{ID: "github", Name: "GitHub", What: "Repositories, issues and pull requests you can see on GitHub", URL: "https://api.githubcopilot.com/mcp/", NeedsClient: true,
		Scopes: []string{"repo", "read:org", "read:user"}},
	{ID: "asana", Name: "Asana", What: "Tasks and projects in your Asana", URL: "https://mcp.asana.com/v2/mcp", NeedsClient: true},
	{ID: "hubspot", Name: "HubSpot", What: "Contacts, companies and deals in your HubSpot", URL: "https://mcp.hubspot.com/", NeedsClient: true},
}

func find(entries []Entry, id string) (Entry, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return Entry{}, false
}

// WithMore is the catalog and more apps besides, each checked: an id that is a name, a name, and an https address, or an http one on
// this computer.
func WithMore(more []Entry) ([]Entry, error) {
	all := slices.Clone(Catalog)
	for _, entry := range more {
		if !job.Named(entry.ID) || entry.Name == "" || !secureAddress(entry.URL) {
			return nil, fmt.Errorf("the app %q needs an id of lower case letters, digits and hyphens, a name, and an https address", entry.ID)
		}
		if _, taken := find(all, entry.ID); taken {
			return nil, fmt.Errorf("there is already a connector called %s", entry.ID)
		}
		all = append(all, entry)
	}
	return all, nil
}
