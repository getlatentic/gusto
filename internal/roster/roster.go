// Package roster is who is in an organisation, kept in a file that its front door
// writes and its server reads. Gusto cloud owns the people, since it signs them
// in and invites them; the server only shows them, so it holds no second copy that
// could disagree.
package roster

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// Member is a person in the organisation. Role is "admin" or "runner".
type Member struct {
	Email      string    `json:"email"`
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

// Roster is the whole list, and when gusto cloud wrote it.
type Roster struct {
	Written time.Time `json:"written"`
	Members []Member  `json:"members"`
	Invited []Invited `json:"invited"`
}

// Write puts the roster where a server reads it.
func Write(path string, r Roster) error { return atomicfile.Write(path, r, 0o600) }

// Read is the roster at path. A file that cannot be read is an error and not an
// empty organisation: a page that showed nobody would be wrong, and would look right.
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
