package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheFirstRuleThatMatchesDecidesAndNoRuleAllows(t *testing.T) {
	policy := Policy{Rules: []Rule{
		{Server: "crm", Tool: "export_*", Person: "*@contractor.example", Effect: "deny", Why: "contractors may not export"},
		{Server: "crm", Tool: "export_*", Effect: "allow"},
		{Tool: "drop_*", Effect: "deny"},
	}}
	cases := []struct {
		person, server, tool string
		allowed              bool
		why                  string
	}{
		{"x@contractor.example", "crm", "export_all", false, "contractors may not export"},
		{"x@acme.example", "crm", "export_all", true, ""},
		{"x@acme.example", "crm", "read", true, ""},
		{"x@acme.example", "db", "drop_table", false, "this tool is not allowed for you"},
	}
	for _, c := range cases {
		allowed, why := policy.Allows(Caller{Person: c.person}, c.server, c.tool)
		if allowed != c.allowed || why != c.why {
			t.Errorf("%+v: got %v %q", c, allowed, why)
		}
	}
}

func TestAPolicyFileIsReadAndABadOneIsRefusedNamingTheRule(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "policy.json")
	os.WriteFile(good, []byte(`{"rules":[{"tool":"x*","effect":"deny"}]}`), 0o644)
	if policy, err := ReadPolicy(good); err != nil || len(policy.Rules) != 1 {
		t.Fatalf("%+v %v", policy, err)
	}
	if policy, err := ReadPolicy(filepath.Join(dir, "none.json")); err != nil || len(policy.Rules) != 0 {
		t.Errorf("a missing file is no rules: %+v %v", policy, err)
	}
	if policy, err := ReadPolicy(""); err != nil || len(policy.Rules) != 0 {
		t.Errorf("no file named: %+v %v", policy, err)
	}
	for name, text := range map[string]string{
		"no effect":      `{"rules":[{"tool":"x"}]}`,
		"a wrong effect": `{"rules":[{"tool":"x","effect":"maybe"}]}`,
		"a bad pattern":  `{"rules":[{"tool":"[","effect":"deny"}]}`,
		"not JSON":       `{`,
	} {
		bad := filepath.Join(dir, "bad.json")
		os.WriteFile(bad, []byte(text), 0o644)
		if _, err := ReadPolicy(bad); err == nil {
			t.Errorf("%s: the policy was taken", name)
		} else if name != "not JSON" && !strings.Contains(err.Error(), "rule 1") {
			t.Errorf("%s: the error does not name the rule: %v", name, err)
		}
	}
}
