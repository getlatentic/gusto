package web

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

// step is one thing an agent did during a turn, said the way a person would:
// "Read adder.sh", "Searched memory for favourite colour".
type step struct {
	What   string
	Detail string
}

// verbs are the tool names harnesses use, by what they do.
var verbs = map[string]string{
	"read": "Read", "read_file": "Read", "view": "Read", "cat": "Read",
	"edit": "Edited", "patch": "Edited", "apply_patch": "Edited", "multiedit": "Edited",
	"write": "Wrote", "write_file": "Wrote", "create": "Wrote",
	"bash": "Ran", "shell": "Ran", "execute": "Ran", "terminal": "Ran", "exec": "Ran", "command": "Ran",
	"glob": "Searched", "grep": "Searched", "search": "Searched", "search_files": "Searched", "list": "Looked in", "ls": "Looked in",
	"fetch": "Fetched", "webfetch": "Fetched", "web_search": "Searched the web",
	"skill": "Used skill", "todowrite": "Planned", "todo": "Planned", "think": "Thought",
}

// stepsOf is what a turn did, in words.
func stepsOf(dir job.Dir) []step {
	var steps []step
	for _, done := range ranBy(dir) {
		steps = append(steps, describe(done))
	}
	return steps
}

func describe(done ran) step {
	tool := strings.ToLower(strings.TrimSpace(done.Tool))
	about := subjectOf(done)
	if server, name, found := strings.Cut(strings.TrimPrefix(tool, "mcp__"), "__"); found && strings.HasPrefix(tool, "mcp__") {
		shown := step{What: capital(server), Detail: words(name)}
		if about != "" {
			shown.Detail += ": " + about
		}
		return shown
	}
	if verb, found := verbs[tool]; found {
		if tool == "skill" {
			about = capital(words(about))
		}
		return step{What: verb, Detail: about}
	}
	return step{What: capital(words(tool)), Detail: about}
}

// subjectOf is what a step was about: the file, the command, the query. A
// harness that titled the step already said it.
func subjectOf(done ran) string {
	var input map[string]any
	if json.Unmarshal(done.Input, &input) == nil {
		for _, key := range []string{"file_path", "filePath", "path", "command", "cmd", "query", "pattern", "name", "url", "names"} {
			if value, found := input[key]; found {
				return tidy(value)
			}
		}
	}
	if done.Title != "" {
		_, after, found := strings.Cut(done.Title, ": ")
		if found {
			return tidy(after)
		}
		return tidy(done.Title)
	}
	var plain string
	if json.Unmarshal(done.Input, &plain) == nil {
		return tidy(plain)
	}
	return ""
}

// tidy is a value as a person reads it: a path inside the agent's folder by
// its name there, a list by its members, and nothing longer than a line.
func tidy(value any) string {
	var text string
	switch held := value.(type) {
	case string:
		text = held
	case []any:
		parts := make([]string, 0, len(held))
		for _, each := range held {
			parts = append(parts, tidy(each))
		}
		text = strings.Join(parts, ", ")
	default:
		encoded, _ := json.Marshal(held)
		text = string(encoded)
	}
	text = strings.TrimPrefix(text, "/job/workspace/")
	if strings.HasPrefix(text, "/job/workspace") {
		text = path.Base(text)
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 120 {
		text = text[:120] + "…"
	}
	return text
}

func words(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "_", " "), "-", " ")
}

func capital(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
