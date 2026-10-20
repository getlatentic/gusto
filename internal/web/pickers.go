package web

import (
	"encoding/json"
	"strings"
)

// option is one choice in a picker: what it is called, and a quieter word on what it costs or does.
type option struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	// Icon is the picture that stands for the choice, when it has one.
	Icon string `json:"icon,omitempty"`
}

func (s *Server) modelOption(model string) option {
	return option{Value: model, Label: modelName(model), Detail: strings.Replace(s.costOf(model), " per million tokens", " per 1M", 1)}
}

// modelOptions are the models of a harness as a picker lists them.
func (s *Server) modelOptionsOf(models []string) []option {
	options := make([]option, 0, len(models))
	for _, model := range models {
		options = append(options, s.modelOption(model))
	}
	return options
}

// agentOptions is the harnesses as JSON for the page's picker.
func (s *Server) agentOptions(picks []pick) string {
	options := make([]option, 0, len(picks))
	for _, each := range picks {
		label := each.Title
		if label == "" {
			label = each.Name
		}
		options = append(options, option{Value: each.Name, Label: label, Icon: logo(each.Name)})
	}
	return asJSON(options)
}

// modelOptions is each harness's models as JSON, so choosing a harness changes what the other picker lists.
func (s *Server) modelOptions(picks []pick) string {
	byHarness := map[string][]option{}
	for _, each := range picks {
		byHarness[each.Name] = s.modelOptionsOf(each.Models)
	}
	return asJSON(byHarness)
}

func asJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(encoded)
}
