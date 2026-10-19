// Package computer is the record of the computers an organisation's people have:
// one microVM for each person, tied to that person and to the organisation. The
// record is what the organisation has, and the gateway's own list is checked
// against it.
package computer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
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

// Computer is one person's, in one organisation.
type Computer struct {
	ID   string `json:"id"`
	Org  string `json:"org"`
	User string `json:"user"`
	// Sandbox is the name the gateway knows it by. It begins with a digest of the
	// organisation, so what is an organisation's on the gateway is found by it.
	Sandbox    string    `json:"sandbox"`
	Host       string    `json:"host"`
	State      State     `json:"state"`
	Created    time.Time `json:"created"`
	LastActive time.Time `json:"lastActive,omitzero"`
	// Note is why a computer failed, or what was last done to it.
	Note string `json:"note,omitempty"`
}

// Live says the computer takes a place in the organisation's limit.
func (c Computer) Live() bool { return c.State != Deleted }

// Marker begins the name of every computer's sandbox, and no job's: a job's
// sandbox is called gusto-<digest>.
const Marker = "c-"

// A gateway takes at most 19 characters in a sandbox's name: the marker, eight
// characters of a digest of the organisation, a hyphen and eight of a digest of the
// organisation and the person. The organisation's digest is the same for all of its
// computers, so what is the organisation's on the gateway is found by it exactly.
func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:8]
}

// Prefix is the organisation's part of its computers' names.
func Prefix(org string) string { return Marker + digest("org\x00"+org) + "-" }

// ID is the computer's own name in the organisation's record. The organisation is in
// what is hashed, so one address in two organisations is two computers.
func ID(org, user string) string { return digest(org + "\x00" + user) }

// Name is the sandbox name of a person's computer in an organisation.
func Name(org, user string) string { return Prefix(org) + ID(org, user) }

// ErrNone is what asking for a computer nobody has made says.
var ErrNone = errors.New("that person has no computer here")
