package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/instance"
)

// A person writes a bio of themselves: what they do, what they are working on, how they like to work. The agents in the rooms they
// are in read it with who else is there, so they can answer to the person asking.

func init() { helpers["maxBio"] = func() int { return instance.MaxBio } }

// briefBios is how many characters of bios one turn's brief holds. People past it are left out, so a big room does not crowd out
// what was said.
const briefBios = 6000

// personBio is a person's bio as their page shows it, and whether the person looking may change it.
type personBio struct {
	Text string
	Mine bool
}

func (s *Server) bioOf(r *http.Request, who string) personBio {
	return personBio{Text: s.instances.Bio(who), Mine: strings.EqualFold(who, s.askerName(r))}
}

// setPersonBio keeps what the person asking wrote of themselves. Nobody writes another person's.
func (s *Server) setPersonBio(w http.ResponseWriter, r *http.Request) {
	who := strings.ToLower(r.PathValue("who"))
	if !strings.EqualFold(who, s.askerName(r)) {
		http.Error(w, "a bio is written by the person it is of", http.StatusForbidden)
		return
	}
	back := "/members/people/" + url.PathEscape(who)
	was := s.instances.Bio(who)
	if err := s.instances.SetBio(who, r.FormValue("bio")); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	if now := s.instances.Bio(who); now != was {
		s.note(r, changelog.PersonBio, map[string]string{"was": was, "now": now})
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// peopleBios is what the room's people said of themselves, as one turn's brief gives it, or nothing when none did.
func (s *Server) peopleBios(held instance.Instance) string {
	var out strings.Builder
	for _, member := range held.Members {
		bio := strings.Join(strings.Fields(s.instances.Bio(member.Who)), " ")
		if bio == "" {
			continue
		}
		line := "- " + s.personName(member.Who) + " (" + member.Who + "): " + bio + "\n"
		if out.Len()+len(line) > briefBios {
			break
		}
		out.WriteString(line)
	}
	if out.Len() == 0 {
		return ""
	}
	return "\nWhat the people here say of themselves:\n" + out.String()
}
