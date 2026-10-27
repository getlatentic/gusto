package web

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
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

// modelOptions is each harness's models as JSON, so choosing a harness changes what the other picker lists. Each model says
// how the agent did with it in the benchmark, where it was measured, so a person can choose with that in front of them.
func (s *Server) modelOptions(picks []pick) string {
	byHarness := map[string][]option{}
	for _, each := range picks {
		options := s.modelOptionsOf(each.Models)
		for i := range options {
			options[i].Detail = joinDetail(options[i].Detail, measuredWords(each.Entry, options[i].Value))
		}
		byHarness[each.Name] = options
	}
	return asJSON(byHarness)
}

// measuredWords is what the benchmark saw this agent do with the model on its simplest task, or nothing when it did not run it.
func measuredWords(entry catalog.Entry, model string) string {
	if run, finished := entry.Finished(model); finished {
		if run.WallSeconds != nil {
			return "finished our test in " + (time.Duration(*run.WallSeconds) * time.Second).String()
		}
		return "finished our test"
	}
	if _, cannot := entry.CannotFinish(model); cannot {
		return "did not finish our test"
	}
	return ""
}

func joinDetail(cost, measured string) string {
	switch {
	case measured == "":
		return cost
	case cost == "":
		return measured
	}
	return cost + " · " + measured
}

func asJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(encoded)
}
