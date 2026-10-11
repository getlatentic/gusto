package web

import (
	"net/http"
	"slices"
	"strings"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/orgsettings"
)

// chatDefault is the engine and model the organisation's admins chose for a new chat, which a person gets until they choose their own.
type chatDefault struct{ Harness, Model string }

func (d chatDefault) words() string {
	if d.Harness == "" {
		return ""
	}
	return strings.TrimSuffix(d.Harness+" "+d.Model, " ")
}

// UseChatDefault is what a new chat runs on when neither the person nor the organisation's admins have chosen: a pair the operator
// knows finishes a job, so that a first chat on a deployment that offers every model to every engine still gets a reply.
func (s *Server) UseChatDefault(harness, model string) {
	s.operatorDefault = chatDefault{Harness: harness, Model: model}
}

// orgDefault is the organisation's default for a new chat, else the operator's.
func (s *Server) orgDefault() chatDefault {
	if s.orgSettings == nil {
		return s.operatorDefault
	}
	settings, err := s.orgSettings.Read()
	if err != nil || settings.DefaultHarness == "" {
		return s.operatorDefault
	}
	return chatDefault{Harness: settings.DefaultHarness, Model: settings.DefaultModel}
}

// defaultChoice is one engine and model an admin may make the default, as the admin overview lists it.
type defaultChoice struct {
	Value, Label string
	Chosen       bool
}

// defaultChoices are the engines and models that can run here, each pair one choice, with the organisation's current default marked.
func (s *Server) defaultChoices(r *http.Request) []defaultChoice {
	now := s.orgDefault()
	var choices []defaultChoice
	for _, each := range s.picksFor(r) {
		engine := each.Title
		if engine == "" {
			engine = each.Name
		}
		for _, model := range each.Models {
			choices = append(choices, defaultChoice{
				Value:  each.Name + "|" + model,
				Label:  engine + " · " + modelName(model),
				Chosen: each.Name == now.Harness && model == now.Model,
			})
		}
	}
	return choices
}

// setChatDefault keeps the engine and model an admin chose for a new chat, given as "engine|model", and clears it when given none.
func (s *Server) setChatDefault(w http.ResponseWriter, r *http.Request) {
	harness, model, _ := strings.Cut(r.FormValue("default"), "|")
	chosen := chatDefault{Harness: harness, Model: model}
	if harness != "" && !s.canRun(r, chosen) {
		s.turnAway(w, r, "/admin", "that engine and model cannot run here")
		return
	}
	var was chatDefault
	err := s.orgSettings.Change(func(settings *orgsettings.Settings) {
		was = chatDefault{Harness: settings.DefaultHarness, Model: settings.DefaultModel}
		settings.DefaultHarness, settings.DefaultModel = chosen.Harness, chosen.Model
	})
	if err != nil {
		s.turnAway(w, r, "/admin", err.Error())
		return
	}
	if was != chosen {
		s.note(r, changelog.ChatDefaultSet, map[string]string{"was": was.words(), "now": chosen.words()})
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) canRun(r *http.Request, d chatDefault) bool {
	for _, each := range s.picksFor(r) {
		if each.Name == d.Harness {
			return slices.Contains(each.Models, d.Model)
		}
	}
	return false
}
