package setup

import (
	"encoding/json"
	"strings"
)

// parse reads a SKILL.md: its front matter's name and description, and what
// follows as the body.
func parse(document string) Skill {
	rest, found := strings.CutPrefix(document, "---\n")
	if !found {
		return Skill{Body: document}
	}
	front, body, found := strings.Cut(rest, "\n---\n")
	if !found {
		return Skill{Body: document}
	}
	skill := Skill{Body: strings.TrimSpace(body)}
	for _, line := range strings.Split(front, "\n") {
		key, value, _ := strings.Cut(line, ":")
		value = unquote(strings.TrimSpace(value))
		switch strings.TrimSpace(key) {
		case "name":
			skill.Name = value
		case "description":
			skill.Description = value
		}
	}
	return skill
}

// quote writes a YAML scalar that survives any text: a JSON string is a YAML
// double-quoted string.
func quote(text string) string {
	quoted, _ := json.Marshal(text)
	return string(quoted)
}

func unquote(text string) string {
	var plain string
	if strings.HasPrefix(text, `"`) && json.Unmarshal([]byte(text), &plain) == nil {
		return plain
	}
	return text
}
