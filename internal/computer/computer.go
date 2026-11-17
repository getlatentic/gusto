// Package computer is the record of the computers an organization's people have:
// one microVM for each person in each team they are in, tied to that person,
// that team and the organization. The record is what the organization has, and
// the gateway's own list is checked against it.
package computer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/team"
)

// State is where a computer is.
type State string

const (
	Creating State = "creating"
	Running  State = "running"
	Stopped  State = "stopped"
	Failed   State = "failed"
	Deleted  State = "deleted"
)

// Computer is one person's, in one team of one organization.
type Computer struct {
	ID   string `json:"id"`
	Org  string `json:"org"`
	User string `json:"user"`
	// Team is the team the computer is in, and empty for the first.
	Team string `json:"workspace,omitempty"`
	// Sandbox is the name the gateway knows it by. It begins with a digest of the
	// organization, so what is an organization's on the gateway is found by it.
	Sandbox    string    `json:"sandbox"`
	Host       string    `json:"host"`
	State      State     `json:"state"`
	Created    time.Time `json:"created"`
	LastActive time.Time `json:"lastActive,omitzero"`
	// Note is why a computer failed, or what was last done to it.
	Note string `json:"note,omitempty"`
}

// Live says the computer takes a place in the organization's limit.
func (c Computer) Live() bool { return c.State != Deleted }

// Marker begins the name of every computer's sandbox, and no task's: a task's
// sandbox is called gusto-<digest>.
const Marker = "c-"

// A gateway takes at most 19 characters in a sandbox's name: the marker, eight
// characters of a digest of the organization, a hyphen and eight of a digest of the
// organization, the team and the person. The organization's digest is the same for
// all of its computers, so what is the organization's on the gateway is found by it exactly.
func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:8]
}

// Prefix is the organization's part of its computers' names.
func Prefix(org string) string { return Marker + digest("org\x00"+org) + "-" }

// ID is the computer's own name in the organization's record. The organization and the
// team are in what is hashed, so one address in two organizations, or in two
// teams, is two computers. A computer in the first team is hashed without it,
// so the computers made before teams keep their names.
func ID(org, space, user string) string {
	if space = team.Of(space); space != team.First {
		return digest(org + "\x00" + space + "\x00" + user)
	}
	return digest(org + "\x00" + user)
}

// Name is the sandbox name of a person's computer in a team.
func Name(org, space, user string) string { return Prefix(org) + ID(org, space, user) }

// ErrNone is what asking for a computer nobody has made says.
var ErrNone = errors.New("that person has no computer here")
