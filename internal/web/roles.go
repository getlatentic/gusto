package web

import (
	"encoding/json"
	"errors"
	"strings"
)

// Role is what somebody may do here. A runner starts agents and comparisons and
// sees their own work. An admin also sees everybody's and changes what agents
// are given.
type Role string

const (
	Runner Role = "runner"
	Admin  Role = "admin"
)

// Person is what the provider says about somebody who signed in.
type Person struct {
	Email  string
	Groups []string
}

// Access is how the provider's answer becomes a role, and so who is let in.
//
// A company's own provider decides who has an account and which groups they
// are in, so the groups it names are what grant a role. A provider that has no
// groups to name, or a public one that lets anybody sign in, is gated by the
// addresses and domains listed in Allowed instead.
type Access struct {
	// Allowed, when it names any, restricts sign-in to those addresses and
	// domains as well as whatever the groups grant.
	Allowed []string
	// RunnerGroups and AdminGroups are the provider's groups that grant each
	// role. An admin group outranks a runner group.
	RunnerGroups []string
	AdminGroups  []string
	// GroupsClaim is where the provider puts somebody's groups. It is "groups"
	// unless said otherwise.
	GroupsClaim string
	// Scopes are asked of the provider besides openid and email, which some
	// providers need before they release groups.
	Scopes []string
}

func (a Access) groupsClaim() string {
	if a.GroupsClaim == "" {
		return "groups"
	}
	return a.GroupsClaim
}

// grants is whether roles come from groups at all.
func (a Access) grants() bool { return len(a.RunnerGroups)+len(a.AdminGroups) > 0 }

// gated is whether anything at all decides who is let in. A provider that lets
// everybody with an account through, and a server that then lets them spend its
// credential, is what this refuses to start as.
func (a Access) gated() bool { return a.grants() || len(a.Allowed) > 0 }

var errUngated = errors.New("name who may sign in, with an allowed list or the groups that grant a role, or anybody with an account at that provider can spend this server's credential")

// admits reports whether this address is one the allowed list lets in. An entry
// is an address, or a domain written with a leading @ that any address at it
// may use.
func (a Access) admits(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	_, domain, found := strings.Cut(email, "@")
	for _, allowed := range a.Allowed {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		switch {
		case allowed == "":
		case allowed == email:
			return true
		case found && strings.HasPrefix(allowed, "@") && allowed[1:] == domain:
			return true
		}
	}
	return false
}

// roleOf is the role somebody signs in with, or that they get none. With groups
// named, a person in none of them is refused; with none named, the allowed list
// is the gate and everybody it lets in is a runner.
func (a Access) roleOf(who Person) (Role, bool) {
	if len(a.Allowed) > 0 && !a.admits(who.Email) {
		return "", false
	}
	switch {
	case inAny(who.Groups, a.AdminGroups):
		return Admin, true
	case inAny(who.Groups, a.RunnerGroups):
		return Runner, true
	case a.grants():
		return "", false
	}
	return Runner, true
}

func inAny(have, want []string) bool {
	for _, group := range have {
		for _, wanted := range want {
			if group != "" && group == strings.TrimSpace(wanted) {
				return true
			}
		}
	}
	return false
}

// groupsIn reads a groups claim, which a provider sends as a list, or as one
// string when somebody is in one group.
func groupsIn(claim json.RawMessage) []string {
	var list []string
	if json.Unmarshal(claim, &list) == nil {
		return list
	}
	var one string
	if json.Unmarshal(claim, &one) == nil && one != "" {
		return []string{one}
	}
	return nil
}
