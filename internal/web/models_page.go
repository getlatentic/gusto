package web

import (
	"net/http"
	"net/url"

	"github.com/getlatentic/gusto/internal/registry"
)

// modelsPath is the console's page of the models the organization may use.
const modelsPath = "/admin/models"

// modelsTab is a tab of the models page, which is where a form on that tab goes back to.
func modelsTab(key string) string { return modelsPath + "?tab=" + key }

// providerPage is one provider on the Providers tab.
func providerPage(name string) string {
	return modelsTab("providers") + "&provider=" + url.QueryEscape(name)
}

// providerRow is one provider as the table of providers reads it.
type providerRow struct {
	Name, Href, Address, Key, State string
	// More is how many addresses it has beside the first.
	More, Models int
	Operator     bool
}

// modelsView is what every tab of the models page reads of the registry: how a provider may be added, and the models a person
// may be limited to. The providers themselves are in the tab that lists them.
type modelsView struct {
	Kinds     []string
	Available []string
	Private   bool
	Editable  bool
	Unnamed   bool
}

func modelsViewOf(view registry.View) modelsView {
	return modelsView{Kinds: view.Kinds, Available: view.Available, Private: view.Private, Editable: view.Editable, Unnamed: view.Unnamed}
}

// modelsTabs are the tabs of the models page. Settings is there when the organization has settings or the registry has a past.
func (s *Server) modelsTabs(r *http.Request, steps []registry.Step) ([]tab, string) {
	tabs := []tab{{Key: "models", Label: "Models"}, {Key: "providers", Label: "Providers"}, {Key: "access", Label: "Who may use"}}
	if s.hasOrgSettings() || len(steps) > 0 {
		tabs = append(tabs, tab{Key: "settings", Label: "Settings"})
	}
	return chooseTab(r, modelsPath, tabs)
}

// modelsData is the tab the request names: the organization's models, its providers or one of them, who may use what, or its settings.
func (s *Server) modelsData(r *http.Request) (map[string]any, error) {
	view := s.registry.View()
	steps := s.registry.Steps()
	tabs, chosen := s.modelsTabs(r, steps)
	page := map[string]any{"View": modelsViewOf(view), "Refused": s.refusal(r), "Tabs": tabs, "Tab": chosen}
	switch chosen {
	case "models":
		page["Rows"], page["Pages"] = onePage(r, modelsTab("models"), s.modelRows(view))
	case "providers":
		s.providersInto(r, view, page)
	case "access":
		page["People"] = s.memberEmails()
		page["Limits"], page["Pages"] = onePage(r, modelsTab("access"), view.Limits)
	case "settings":
		page["Steps"], page["Pages"] = onePage(r, modelsTab("settings"), steps)
		if s.hasOrgSettings() {
			choices := s.defaultChoices(r)
			page["Defaults"], page["DefaultNow"], page["RunTokens"] = choices, chatDefaultWords(choices), s.runTokensOf()
		}
	}
	return page, nil
}

// providersInto fills the Providers tab: the provider the request names, with its models, or the table of them all.
func (s *Server) providersInto(r *http.Request, view registry.View, page map[string]any) {
	page["Sources"] = keySources(view)
	name := r.URL.Query().Get("provider")
	for _, provider := range view.Providers {
		if provider.Retired || provider.Name != name {
			continue
		}
		var models []registry.ModelView
		for _, model := range provider.Models {
			if !model.Retired {
				models = append(models, model)
			}
		}
		page["Provider"] = provider
		page["Models"], page["Pages"] = onePage(r, providerPage(provider.Name), models)
		return
	}
	page["Providers"] = providerRows(view)
}

func providerRows(view registry.View) []providerRow {
	var rows []providerRow
	for _, provider := range view.Providers {
		if provider.Retired {
			continue
		}
		row := providerRow{Name: provider.Name, Href: providerPage(provider.Name), Key: keyWords(provider.Key), State: providerState(provider),
			Operator: provider.Operator}
		if len(provider.Addresses) > 0 {
			row.Address, row.More = provider.Addresses[0].Address, len(provider.Addresses)-1
		}
		for _, model := range provider.Models {
			if !model.Retired {
				row.Models++
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func keyWords(key string) string {
	if key == "stored" {
		return "kept here"
	}
	return key
}

func providerState(provider registry.ProviderView) string {
	switch {
	case provider.Problem != "":
		return "not in use"
	case provider.Pending:
		return "off"
	}
	return "on"
}
