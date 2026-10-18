package desk

import (
	"strings"

	"github.com/getlatentic/gusto/internal/helper"
)

const briefWidth = 48

// short is a brief on one line that fits a menu.
func short(brief string) string {
	line := strings.Join(strings.Fields(brief), " ")
	if line == "" {
		return "a turn"
	}
	runes := []rune(line)
	if len(runes) <= briefWidth {
		return line
	}
	return string(runes[:briefWidth-1]) + "…"
}

// stateWord is a turn's state the way the web says it to a person.
func stateWord(state string) string {
	switch state {
	case "queued":
		return "waiting"
	case "running":
		return "working"
	case "passed":
		return "finished"
	case "failed":
		return "failed"
	case "interrupted":
		return "stopped"
	}
	return state
}

func ended(state string) bool {
	return state == "passed" || state == "failed" || state == "interrupted"
}

// PageOf is the address of the page that shows a turn: its conversation, or the task when it belongs to none.
func PageOf(server string, turn helper.Turn) string {
	server = strings.TrimRight(server, "/")
	if turn.Instance != "" && turn.Thread != "" {
		return server + "/instances/" + turn.Instance + "/threads/" + turn.Thread
	}
	return server + "/jobs/" + turn.Job
}
