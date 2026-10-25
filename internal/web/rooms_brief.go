package web

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/getlatentic/gusto/internal/instance"
)

// roomBrief is what one of a room's agents is given for a turn: who it is and who else is in the room, what was said since it
// last spoke, and what it is asked now.
func (s *Server) roomBrief(held instance.Instance, thread instance.Thread, agent instance.Agent, asked, said, from string) string {
	var brief strings.Builder
	role := ""
	if agent.Job != "" {
		role = ", for " + agent.Job
	}
	fmt.Fprintf(&brief, "You are %s%s, an agent in the room %q. People and agents share this room and its folder.\n", agent.Name, role, held.Title)
	if persona := strings.TrimSpace(agent.Persona); persona != "" {
		brief.WriteString("\nHow you work, as your team set it:\n" + persona + "\n\n")
	}
	brief.WriteString("People here:")
	for _, member := range held.Members {
		fmt.Fprintf(&brief, " %s (%s);", member.Who, member.Role)
	}
	brief.WriteString("\nAgents here:")
	for _, each := range held.Agents {
		if each.ID == agent.ID {
			continue
		}
		fmt.Fprintf(&brief, " @%s (%s, %s);", each.Name, firstOf(each.Job, "no job named"), s.engineName(each.Harness))
	}
	brief.WriteString("\nTo hand work to another agent, mention it as @Name in your reply, only when it should act now. What you reply is posted to the room.\n")
	if earlier := s.sinceLastSpoke(held, thread, agent); len(earlier) > 0 {
		brief.WriteString("\nSaid in the room since you last spoke:\n")
		for _, line := range earlier {
			brief.WriteString(line + "\n")
		}
	}
	speaker := asked
	if from != "" {
		if handing, found := held.AgentByID(from); found {
			speaker = "@" + handing.Name
		}
	}
	fmt.Fprintf(&brief, "\n%s says to you:\n%s\n", speaker, said)
	return brief.String()
}

// sinceLastSpoke is what was said in a room after an agent's last turn there, by people and the other agents, oldest first.
func (s *Server) sinceLastSpoke(held instance.Instance, thread instance.Thread, agent instance.Agent) []string {
	type line struct {
		at   int64
		text string
	}
	var last int64
	var lines []line
	for _, id := range thread.Turns {
		record, found := s.index.Get(id)
		if !found {
			continue
		}
		if record.Agent == agent.ID {
			last = max(last, record.Queued)
			continue
		}
		if record.Hops == 0 {
			lines = append(lines, line{record.Queued, record.Asked + ": " + clip(record.Brief)})
		}
		if reply := strings.TrimSpace(said(record.Dir(s.index.Root()))); reply != "" {
			name := record.Agent
			if other, found := held.AgentByID(record.Agent); found {
				name = other.Name
			}
			lines = append(lines, line{max(record.Ended, record.Queued), "@" + name + ": " + clip(reply)})
		}
	}
	for _, note := range thread.Notes {
		lines = append(lines, line{note.At, note.Who + ": " + clip(note.Text)})
	}
	lines = slices.DeleteFunc(lines, func(each line) bool { return each.at < last })
	slices.SortStableFunc(lines, func(a, b line) int { return int(a.at - b.at) })
	if len(lines) > sinceLast {
		lines = lines[len(lines)-sinceLast:]
	}
	out := make([]string, 0, len(lines))
	for _, each := range lines {
		out = append(out, each.text)
	}
	return out
}

// clip is a message cut to what an agent is told of it.
func clip(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= heard {
		return text
	}
	cut := heard
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
