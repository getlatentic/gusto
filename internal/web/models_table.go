package web

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/pkg/spend"
)

// modelRow is one model the organization has, as the table at the top of the models page reads it.
type modelRow struct {
	Name, Full, Provider string
	Input, Output, Who   string
	// Off is a model, or a provider of it, that was added and is waiting to be turned on.
	Off bool
}

// modelRows are the models of the providers in use, each with its price and who may use it.
func (s *Server) modelRows(view registry.View) []modelRow {
	var rows []modelRow
	for _, provider := range view.Providers {
		if provider.Retired {
			continue
		}
		for _, model := range provider.Models {
			if model.Retired {
				continue
			}
			row := modelRow{Name: modelName(model.Full), Full: model.Full, Provider: provider.Name, Off: provider.Pending || model.Pending,
				Who: s.whoMayUse(model.Full, view.Limits)}
			row.Input, row.Output = perMillion(model.Price)
			rows = append(rows, row)
		}
	}
	return rows
}

func perMillion(price *spend.Price) (input, output string) {
	if price == nil {
		return "", ""
	}
	return fmt.Sprintf("$%.2f", price.InputPerMillion), fmt.Sprintf("$%.2f", price.OutputPerMillion)
}

// whoMayUse says who may use a model: everyone, but for the people limited to models that leave it out.
func (s *Server) whoMayUse(model string, limits []registry.Access) string {
	var not []string
	for _, limit := range limits {
		if !slices.Contains(limit.Models, model) {
			not = append(not, s.personName(limit.Email))
		}
	}
	switch len(not) {
	case 0:
		return "everyone"
	case 1:
		return "everyone but " + not[0]
	case 2:
		return "everyone but " + not[0] + " and " + not[1]
	}
	return "everyone but " + strconv.Itoa(len(not)) + " people"
}

// chatDefaultWords is what a new chat runs on, as the models page says it.
func chatDefaultWords(choices []defaultChoice) string {
	for _, choice := range choices {
		if choice.Chosen {
			return choice.Label
		}
	}
	return "The first agent and model that can run"
}

// settingsBack is the page the forms for a new chat's default and the sandboxes' tokens are on.
func (s *Server) settingsBack() string {
	if s.hasRegistry() {
		return modelsTab("settings")
	}
	return "/admin"
}
