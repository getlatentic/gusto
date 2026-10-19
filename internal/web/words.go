package web

import (
	"strings"

	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
)

// The pages are for people who do not know what a harness, a model id or a
// credential is. This is where the machine's names become theirs.

// agent is an instance as a person sees it.
type agent struct {
	instance.Instance
	// Name is what it is called, Field the kind of work it was set up for,
	// empty for one started blank, and Engine and Brain the program and the
	// model in words.
	Name   string
	Field  string
	Engine string
	Brain  string
}

// Runs is the engine and the model in one line, for the place under its name.
// An agent already named for its engine is not told its name twice.
func (a agent) Runs() string {
	if a.Engine == "" || a.Engine == a.Name {
		return a.Brain
	}
	if a.Brain == "" {
		return a.Engine
	}
	return a.Engine + " · " + a.Brain
}

func (s *Server) agentOf(held instance.Instance) agent {
	shown := agent{Instance: held, Name: held.Title, Engine: s.engineName(held.Harness), Brain: modelName(held.Model)}
	if template, found := industry.Get(held.Setup); found {
		shown.Field = template.Industry
		if shown.Name == "" {
			shown.Name = template.Title
		}
	}
	if shown.Name == "" {
		shown.Name = shown.Engine
	}
	if shown.Name == "" {
		shown.Name = "Agent"
	}
	return shown
}

// engineName is the program's name for a person.
func (s *Server) engineName(harness string) string {
	for _, entry := range s.engines() {
		if entry.Name == harness && entry.Title != "" {
			return entry.Title
		}
	}
	return harness
}

// knownModels are the names providers give their models in their own
// catalogs, for the ids gusto is configured with.
var knownModels = map[string]string{
	"deepseek-flash":                   "DeepSeek Flash",
	"deepseek-v4-pro":                  "DeepSeek Pro",
	"deepseek-v4-flash":                "DeepSeek V4 Flash",
	"qwen.qwen3-vl-235b-a22b-instruct": "Qwen 3 VL",
}

// modelName is a model as a person reads it: which credential serves it is
// the operator's business, and an id is spelled out in words.
func modelName(model string) string {
	if model == "" || strings.Contains(model, " ") {
		return model
	}
	if _, alone, found := strings.Cut(model, "/"); found {
		model = alone
	}
	if known, found := knownModels[model]; found {
		return known
	}
	if _, alone, found := strings.Cut(model, "."); found {
		model = alone
	}
	words := strings.FieldsFunc(model, func(r rune) bool { return r == '-' || r == '_' || r == ':' })
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

// fieldClass is the color a field's work is drawn in.
func fieldClass(field string) string {
	if field == "" {
		return "hue-general"
	}
	return "hue-" + strings.ToLower(field)
}

// initial is the first letter of a name, for a mark.
func initial(name string) string {
	for _, letter := range name {
		return strings.ToUpper(string(letter))
	}
	return "·"
}
