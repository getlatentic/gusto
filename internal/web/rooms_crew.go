package web

import (
	"html/template"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/instance"
)

// A room's agents are workspace agents. The room keeps which ones it holds, and takes their name, agent type, model, skills and
// persona from the workspace each time, so a change made to an agent reaches every room it is in.

// liveRoom is a room with each of its agents as the workspace has it now. An agent the workspace no longer has stays as the room
// last knew it, so what it said still reads.
func (s *Server) liveRoom(held instance.Instance) instance.Instance {
	if s.crew == nil {
		return held
	}
	held.Agents = slices.Clone(held.Agents)
	for i, agent := range held.Agents {
		if member, found := s.crew.Get(agent.Member); found {
			held.Agents[i] = liveAgent(agent, member)
		}
	}
	return held
}

func liveAgent(agent instance.Agent, member crew.Agent) instance.Agent {
	agent.Name, agent.Harness, agent.Model, agent.Job = member.Name, member.Harness, member.Model, member.Specialty
	agent.Setup, agent.Persona = member.Setup, member.Persona
	return agent
}

// ownerOf is who made a room's agent, as the workspace has it now, or nobody for an agent the workspace no longer keeps or one
// kept from before agents had owners.
func (s *Server) ownerOf(agent instance.Agent) string {
	if s.crew == nil {
		return ""
	}
	member, _ := s.crew.Get(agent.Member)
	return member.Owner
}

// ownerLabel is whose an agent is, as the person looking reads it.
func (s *Server) ownerLabel(owner, you string) string {
	if owner == "" {
		return ""
	}
	if strings.EqualFold(owner, you) {
		return "your agent"
	}
	return s.personName(owner) + "'s agent"
}

// crewFor is every workspace agent the person asking may work with.
func (s *Server) crewFor(who string) []crew.Agent {
	if s.crew == nil {
		return nil
	}
	var mine []crew.Agent
	for _, member := range s.crew.List() {
		if member.MayUse(who) {
			mine = append(mine, member)
		}
	}
	return mine
}

// span is where a mention of a name sits in a text.
type span struct {
	start, end int
	name       string
}

// mentionSpans is each @ and name in the text, for the names given, the longest name winning where two start at the same @, so
// "@Ada Lovelace" is not also "@Ada". A name counts only where it is not part of a longer word.
func mentionSpans(text string, names []string) []span {
	sorted := slices.Clone(names)
	slices.SortFunc(sorted, func(a, b string) int { return len(b) - len(a) })
	lower := strings.ToLower(text)
	var found []span
	for at := strings.IndexByte(text, '@'); at >= 0; {
		if at == 0 || !wordy(lastRune(text[:at])) {
			for _, name := range sorted {
				end := at + 1 + len(name)
				if name != "" && end <= len(text) && lower[at+1:end] == strings.ToLower(name) && (end == len(text) || !wordy(firstRune(text[end:]))) {
					found = append(found, span{at, end, name})
					break
				}
			}
		}
		next := strings.IndexByte(text[at+1:], '@')
		if next < 0 {
			break
		}
		at += 1 + next
	}
	return found
}

func wordy(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '@' }

func lastRune(text string) rune { r, _ := utf8.DecodeLastRuneInString(text); return r }

func firstRune(text string) rune { r, _ := utf8.DecodeRuneInString(text); return r }

// mentionsIn is a message as a room shows it, each mention of somebody in the room set apart.
func mentionsIn(text string, names []string) template.HTML {
	var shown strings.Builder
	last := 0
	for _, each := range mentionSpans(text, names) {
		shown.WriteString(template.HTMLEscapeString(text[last:each.start]))
		shown.WriteString(`<span class="text-accent">` + template.HTMLEscapeString(text[each.start:each.end]) + `</span>`)
		last = each.end
	}
	shown.WriteString(template.HTMLEscapeString(text[last:]))
	return template.HTML(shown.String())
}

// roomNames is everybody a message in the room can mention: its agents and its people.
func (s *Server) roomNames(held instance.Instance) []string {
	names := make([]string, 0, len(held.Agents)+len(held.Members))
	for _, agent := range held.Agents {
		names = append(names, agent.Name)
	}
	for _, member := range held.Members {
		names = append(names, s.personName(member.Who))
	}
	return names
}
