package web

import (
	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/setup"
)

// skillChoice is a set of skills an agent can be given: one of the ready-made roles, or a skill set somebody made.
type skillChoice struct {
	Name  string
	Title string
	Role  bool
}

func (s *Server) skillChoices() []skillChoice {
	var choices []skillChoice
	roles := map[string]bool{}
	for _, role := range industry.All() {
		roles[role.Name] = true
		choices = append(choices, skillChoice{Name: role.Name, Title: role.Title, Role: true})
	}
	for _, held := range s.setupsList() {
		if !roles[held.Name] {
			choices = append(choices, skillChoice{Name: held.Name, Title: held.Name})
		}
	}
	return choices
}

// skillsOf is the skill set an agent is given, with its skills, or nothing when it has none.
func (s *Server) skillsOf(name string) (setup.Setup, bool) {
	if name == "" || s.setups == nil {
		return setup.Setup{}, false
	}
	return s.setups.Get(name)
}

func (s *Server) skillTitle(name string) string {
	for _, choice := range s.skillChoices() {
		if choice.Name == name {
			return choice.Title
		}
	}
	return name
}
