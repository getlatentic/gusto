// Package team names the teams of an organization, such as Engineering or Business. An organization holds billing, seats
// and admins; a team holds its own members, tasks, records, agents, connectors and computers, which only its members reach.
// What is kept on disk names a task's team under the key workspace, which teams were called when those records began.
package team

import (
	"errors"
	"regexp"
	"strings"
)

// First is the team every organization has. Whatever was kept before teams names none, and is in it.
const First = "main"

// Of is the team a stored record is in: the one it names, or First for one that names none.
func Of(id string) string {
	if id == "" {
		return First
	}
	return id
}

// Stored is how a record names its team: not at all for the first, as the records made before teams do.
func Stored(id string) string {
	if id = Of(id); id != First {
		return id
	}
	return ""
}

// Same says whether two records are in one team.
func Same(a, b string) bool { return Of(a) == Of(b) }

// MaxPerOrg is the most teams an organization may have.
const MaxPerOrg = 50

var shape = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$`)

// ErrBadID is the refusal of a name that cannot be a team's.
var ErrBadID = errors.New("a team is named with lowercase letters, digits and hyphens, at most 32 of them")

// Valid says whether id can name a team.
func Valid(id string) error {
	if !shape.MatchString(id) || strings.Contains(id, "--") {
		return ErrBadID
	}
	return nil
}

// IDFor is the id a team called name is given: its words in lowercase, joined by hyphens. A name with no letter
// or digit in it gives "team".
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
		return "team"
	}
	return id
}
