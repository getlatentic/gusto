package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/preset"
)

// UsePresets is where what people save to start a chat the same way again is kept.
func (s *Server) UsePresets(store *preset.Store) { s.presets = store }

// savedBy is the presets of the person asking, the newest first.
func (s *Server) savedBy(r *http.Request) []preset.Preset {
	if s.presets == nil {
		return nil
	}
	return s.presets.List(s.keeper.who(r))
}

// roleOf is the ready-made kind of work a chat is asked to be given the skills of, when it names one this deployment knows.
func (s *Server) roleOf(r *http.Request) *industry.Template {
	name := strings.TrimSpace(r.FormValue("role"))
	if name == "" {
		return nil
	}
	if held, found := industry.Get(name); found {
		return &held
	}
	return nil
}

// roleChoice is one entry in the chat's role menu, and what choosing it sets.
type roleChoice struct {
	// Kind is "heading" or "separator" for what is not a choice, and "action" for what does something and is not kept.
	Kind    string `json:"kind,omitempty"`
	Value   string `json:"value"`
	Label   string `json:"label"`
	Detail  string `json:"detail,omitempty"`
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Role    string `json:"role,omitempty"`
}

// roleChoices is what the role menu offers: no role, what the person saved, then each ready-made kind of work.
func (s *Server) roleChoices(r *http.Request) string {
	choices := []roleChoice{{Value: "", Label: "No preset"}}
	if saved := s.savedBy(r); len(saved) > 0 {
		choices = append(choices, roleChoice{Kind: "heading", Label: "Your presets"})
		for _, each := range saved {
			choices = append(choices, roleChoice{Value: "p:" + each.ID, Label: each.Name, Detail: s.presetSummary(each), Harness: each.Harness, Model: each.Model, Role: each.Role})
		}
	}
	choices = append(choices, roleChoice{Kind: "heading", Label: "Built-in presets"})
	for _, held := range industry.All() {
		choices = append(choices, roleChoice{Value: "r:" + held.Name, Label: held.Title, Detail: held.Industry, Role: held.Name})
	}
	if s.presets != nil {
		choices = append(choices, roleChoice{Kind: "separator"}, roleChoice{Kind: "action", Value: "save", Label: "Save this setup as a preset…"})
	}
	return asJSON(choices)
}

// presetSummary says what a preset holds in a sentence: "Software engineer on Claude Code with Claude Sonnet".
func (s *Server) presetSummary(saved preset.Preset) string {
	summary := s.engineTitle(saved.Harness)
	if held, found := industry.Get(saved.Role); found {
		summary = held.Title + " on " + summary
	}
	if saved.Model != "" {
		summary += " with " + s.modelOption(saved.Model).Label
	}
	return summary
}

// chosenRole is the entry the chat opens on: the preset or the role the link named.
func (s *Server) chosenRole(r *http.Request) string {
	if id := r.URL.Query().Get("preset"); id != "" && s.presets != nil {
		if _, found := s.presets.Get(s.keeper.who(r), id); found {
			return "p:" + id
		}
	}
	if name := r.URL.Query().Get("role"); name != "" {
		if _, found := industry.Get(name); found {
			return "r:" + name
		}
	}
	return ""
}

// engineTitle is what an engine is called to a person.
func (s *Server) engineTitle(name string) string {
	for _, entry := range s.engines() {
		if entry.Name == name && entry.Title != "" {
			return entry.Title
		}
	}
	return name
}

// savePreset keeps the engine, the model and the role a chat was about to start with, under a name.
func (s *Server) savePreset(w http.ResponseWriter, r *http.Request) {
	back := "/chat"
	if s.presets == nil {
		http.Redirect(w, r, back+"?refused="+url.QueryEscape("presets cannot be saved here"), http.StatusSeeOther)
		return
	}
	harness := strings.TrimSpace(r.FormValue("harness"))
	if !slices.ContainsFunc(s.picksFor(r), func(each pick) bool { return each.Name == harness }) {
		http.Redirect(w, r, back+"?refused="+url.QueryEscape("choose an engine you may use before saving"), http.StatusSeeOther)
		return
	}
	role := strings.TrimSpace(r.FormValue("role"))
	if _, found := industry.Get(role); role != "" && !found {
		role = ""
	}
	saved, err := s.presets.Save(s.keeper.who(r), preset.Preset{Name: r.FormValue("name"), Harness: harness, Model: strings.TrimSpace(r.FormValue("model")), Role: role})
	if err != nil {
		http.Redirect(w, r, back+"?refused="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, back+"?preset="+saved.ID, http.StatusSeeOther)
}

// deletePreset removes one of the asking person's presets.
func (s *Server) deletePreset(w http.ResponseWriter, r *http.Request) {
	if s.presets != nil {
		if _, err := s.presets.Delete(s.keeper.who(r), r.PathValue("id")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	http.Redirect(w, r, "/new", http.StatusSeeOther)
}

// presetRow is a saved preset as the presets page says it.
type presetRow struct{ ID, Name, Line string }

// presetRows is what the person saved, in words.
func (s *Server) presetRows(r *http.Request) []presetRow {
	var rows []presetRow
	for _, saved := range s.savedBy(r) {
		rows = append(rows, presetRow{ID: saved.ID, Name: saved.Name, Line: s.presetSummary(saved)})
	}
	return rows
}
