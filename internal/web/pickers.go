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
		return finishedWords(run)
	}
	if _, cannot := entry.CannotFinish(model); cannot {
		return "did not finish our test"
	}
	return ""
}

func finishedWords(run catalog.Measured) string {
	if run.WallSeconds != nil {
		return "finished our test in " + (time.Duration(*run.WallSeconds) * time.Second).String()
	}
	return "finished our test"
}

// kind is an agent type as the new agent dialog offers it, with what the benchmark saw it do with the models offered here.
// Every type that can run is offered: Failed only says plainly that it did not finish on any of these it was tried with.
type kind struct {
	catalog.Entry
	Measured string
	Failed   bool
}

func kindsFor(entries []catalog.Entry, offered []string) []kind {
	kinds := make([]kind, len(entries))
	for i, entry := range entries {
		kinds[i] = kind{Entry: entry}
		kinds[i].Measured, kinds[i].Failed = measuredOn(entry, offered)
	}
	return kinds
}

// measuredOn is the quickest the agent finished the benchmark's simplest task on a model offered here, or that it finished on
// none of the offered models it was tried with. It says nothing of an agent never tried with them.
func measuredOn(entry catalog.Entry, offered []string) (string, bool) {
	var fastest *catalog.Measured
	failed := false
	for _, model := range offered {
		run, finished := entry.Finished(model)
		switch {
		case finished && (fastest == nil || quicker(run, *fastest)):
			fastest = &run
		case !finished:
			_, cannot := entry.CannotFinish(model)
			failed = failed || cannot
		}
	}
	switch {
	case fastest != nil:
		words := finishedWords(*fastest)
		return strings.ToUpper(words[:1]) + words[1:], false
	case failed:
		return "Did not finish our test on the models here", true
	}
	return "", false
}

func quicker(a, b catalog.Measured) bool {
	return a.WallSeconds != nil && (b.WallSeconds == nil || *a.WallSeconds < *b.WallSeconds)
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
