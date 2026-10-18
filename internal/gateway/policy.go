package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
)

// Caller is who a task's calls are made for.
type Caller struct {
	Job    string
	Person string
	Team   string
}

// Rule allows or refuses the tools that match it. A field left out matches anything. Server and Tool are patterns as the path
// package reads them (* and ?), Person and Team too, so "*@acme.example" is everyone at a company.
type Rule struct {
	Server string `json:"server,omitempty"`
	Tool   string `json:"tool,omitempty"`
	Person string `json:"person,omitempty"`
	Team   string `json:"team,omitempty"`
	Effect string `json:"effect"`
	// Why is said to the agent and written in the record when a call is refused by this rule.
	Why string `json:"why,omitempty"`
}

// Policy is the rules in the order they were written. The first rule that matches a call decides it, and a call no rule matches is
// allowed, because a server is in a task only when an administrator turned its integration on; the rules take tools away from
// people, or give the tools of a server to only some.
type Policy struct {
	Rules []Rule `json:"rules"`
}

// ReadPolicy reads a policy file. No file is a policy with no rules.
func ReadPolicy(file string) (Policy, error) {
	if file == "" {
		return Policy{}, nil
	}
	text, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return Policy{}, nil
	}
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := json.Unmarshal(text, &policy); err != nil {
		return Policy{}, fmt.Errorf("%s is not a tool policy: %w", file, err)
	}
	return policy, policy.check(file)
}

func (p Policy) check(file string) error {
	for i, rule := range p.Rules {
		if rule.Effect != "allow" && rule.Effect != "deny" {
			return fmt.Errorf("%s: rule %d has the effect %q, and it must be allow or deny", file, i+1, rule.Effect)
		}
		for _, pattern := range []string{rule.Server, rule.Tool, rule.Person, rule.Team} {
			if _, err := path.Match(pattern, ""); err != nil {
				return fmt.Errorf("%s: rule %d has the pattern %q, which is not one: %w", file, i+1, pattern, err)
			}
		}
	}
	return nil
}

// Allows says whether the caller may use a tool of a server, and when not, what to say.
func (p Policy) Allows(caller Caller, server, tool string) (bool, string) {
	for _, rule := range p.Rules {
		if matches(rule.Server, server) && matches(rule.Tool, tool) && matches(rule.Person, caller.Person) && matches(rule.Team, caller.Team) {
			if rule.Effect == "allow" {
				return true, ""
			}
			if rule.Why != "" {
				return false, rule.Why
			}
			return false, "this tool is not allowed for you"
		}
	}
	return true, ""
}

func matches(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	ok, err := path.Match(pattern, value)
	return err == nil && ok
}
