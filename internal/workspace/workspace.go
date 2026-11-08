// Package workspace names a team's space in an organization. An organization holds billing, seats and admins; a
// workspace holds its own members, tasks, records, agents, connectors and computers, which only its members reach.
package workspace

import (
	"errors"
	"regexp"
	"strings"
)

// First is the workspace every organization has. Whatever was kept before workspaces names none, and is in it.
const First = "main"

// Of is the workspace a stored record is in: the one it names, or First for one that names none.
func Of(id string) string {
	if id == "" {
		return First
	}
	return id
}

// Stored is how a record names its workspace: not at all for the first, as the records made before workspaces do.
func Stored(id string) string {
	if id = Of(id); id != First {
		return id
	}
	return ""
}

// Same says whether two records are in one workspace.
func Same(a, b string) bool { return Of(a) == Of(b) }

// MaxPerOrg is the most workspaces an organization may have.
const MaxPerOrg = 50

var shape = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$`)

// ErrBadID is the refusal of a name that cannot be a workspace's.
var ErrBadID = errors.New("a workspace is named with lowercase letters, digits and hyphens, at most 32 of them")

// Valid says whether id can name a workspace.
func Valid(id string) error {
	if !shape.MatchString(id) || strings.Contains(id, "--") {
		return ErrBadID
	}
	return nil
}

// IDFor is the id a workspace called name is given: its words in lowercase, joined by hyphens. A name with no letter
// or digit in it gives "workspace".
func IDFor(name string) string {
	var words []string
	for _, word := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		words = append(words, word)
	}
	id := strings.Join(words, "-")
	if len(id) > 32 {
		id = strings.TrimRight(id[:32], "-")
	}
	if id == "" {
		return "workspace"
	}
	return id
}
