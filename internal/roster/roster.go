// Package roster is who is in an organization, kept in a file that its gusto cloud
// writes and its server reads. Gusto cloud owns the people, since it signs them
// in and invites them; the server only shows them, so it holds no second copy that
// could disagree.
package roster

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
	"github.com/getlatentic/gusto/internal/workspace"
)

// Member is a person in the organization. Role is "admin" or "runner".
type Member struct {
	Email string `json:"email"`
	// Name is what the person said they are called, when they said; their address stands for them otherwise.
	Name       string    `json:"name,omitempty"`
	Role       string    `json:"role"`
	Joined     time.Time `json:"joined"`
	LastSignIn time.Time `json:"lastSignIn,omitzero"`
}

// Invited is somebody who has been asked to join and has not answered.
type Invited struct {
	Email   string    `json:"email"`
	By      string    `json:"by"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}

// Workspace is a team's space in the organization, and the addresses of the people in it.
type Workspace struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

// Roster is the whole list, and when gusto cloud wrote it.
type Roster struct {
	Written    time.Time   `json:"written"`
	Members    []Member    `json:"members"`
	Invited    []Invited   `json:"invited"`
	Workspaces []Workspace `json:"workspaces,omitempty"`
}

// Spaces is the organization's workspaces. A roster that names none has the first, with everybody in it.
func (r Roster) Spaces() []Workspace {
	if len(r.Workspaces) > 0 {
		return r.Workspaces
	}
	first := Workspace{ID: workspace.First, Members: make([]string, 0, len(r.Members))}
	for _, member := range r.Members {
		first.Members = append(first.Members, member.Email)
	}
	return []Workspace{first}
}

// SpacesOf is the workspaces a person is in, in the roster's order.
func (r Roster) SpacesOf(email string) []Workspace {
	var in []Workspace
	for _, space := range r.Spaces() {
		if slices.ContainsFunc(space.Members, func(member string) bool { return strings.EqualFold(member, email) }) {
			in = append(in, space)
		}
	}
	return in
}

// Write puts the roster where a server reads it.
func Write(path string, r Roster) error { return atomicfile.Write(path, r, 0o600) }

// Read is the roster at path. A file that cannot be read is an error and not an
// empty organization: a page that showed nobody would be wrong, and would look right.
func Read(path string) (Roster, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return Roster{}, fmt.Errorf("the list of people in %s cannot be read: %w", path, err)
	}
	var r Roster
	if err := json.Unmarshal(text, &r); err != nil {
		return Roster{}, fmt.Errorf("the list of people in %s is not valid: %w", path, err)
	}
	return r, nil
}
