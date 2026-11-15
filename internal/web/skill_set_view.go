package web

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/getlatentic/gusto/internal/setup"
)

// skillSetView is one skill set as its console page shows it, with each skill's instructions drawn from their Markdown.
type skillSetView struct {
	Name   string
	Off    bool
	Skills []skillView
}

type skillView struct {
	Name, Description string
	// Instructions is the skill's body as HTML, drawn as an agent's reply is, so none of what was written runs.
	Instructions string
}

// serverChoice is an MCP server this deployment offers, where it runs or is reached, and whether the skill set is given it.
type serverChoice struct {
	Name, Where string
	Chosen      bool
}

// earlierRow is one kept version of a skill. At is a string because it counts nanoseconds, past what a page's numbers hold exactly.
type earlierRow struct {
	Skill, At, Fingerprint string
	Present                bool
}

// skillSetData is one skill set, off ones too, and what an admin may give it.
func (s *Server) skillSetData(r *http.Request) (map[string]any, error) {
	held, found := s.setups.Get(r.PathValue("name"))
	if !found {
		return nil, failed{status: http.StatusNotFound, why: "there is no such skill set"}
	}
	uses := held.Uses
	if uses == nil {
		uses = []setup.Use{}
	}
	return map[string]any{
		"Set":      viewOfSkillSet(held),
		"Uses":     uses,
		"Servers":  s.serverChoices(held.Servers),
		"Imported": importedChoices(s.importedIntegrations(), held.Integrations),
		"Earlier":  earlierRows(s.setups.Earlier(held.Name)),
		"Refused":  s.refusal(r),
	}, nil
}

func viewOfSkillSet(held setup.Setup) skillSetView {
	view := skillSetView{Name: held.Name, Off: held.Off, Skills: []skillView{}}
	for _, skill := range held.Skills {
		view.Skills = append(view.Skills, skillView{Name: skill.Name, Description: skill.Description, Instructions: string(markdown(skill.Body))})
	}
	return view
}

func (s *Server) serverChoices(chosen []string) []serverChoice {
	choices := []serverChoice{}
	for _, name := range s.serverNames() {
		server := s.servers[name]
		where := server.URL
		if where == "" {
			where = server.Command
		}
		choices = append(choices, serverChoice{Name: name, Where: where, Chosen: slices.Contains(chosen, name)})
	}
	return choices
}

func earlierRows(kept []setup.Earlier) []earlierRow {
	rows := []earlierRow{}
	for _, each := range kept {
		rows = append(rows, earlierRow{Skill: each.Skill, At: strconv.FormatInt(each.At, 10), Fingerprint: each.Fingerprint, Present: each.Present})
	}
	return rows
}
