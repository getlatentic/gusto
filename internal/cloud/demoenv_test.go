package cloud

import "testing"

// A demo's agents run in a sandbox of another kind than a sign-up's: the demo's settings are given to a demo's server, on top of
// the operator's, and a sign-up's server never has them.
func TestADemoServerHasTheDemoSettingsOnTopAndASignUpDoesNot(t *testing.T) {
	store := open(t, demoSet)
	_, signup, err := store.SignUp(Signup{Email: "ada@acme.com", OrgName: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	_, demo, err := store.StartDemo("Bea", "bea@globex.com", "Globex")
	if err != nil {
		t.Fatal(err)
	}
	c := &Controller{Store: store, Cell: &LocalCell{},
		Env:     []string{"GUSTO_SANDBOX=container", "GUSTO_MODELS=m"},
		DemoEnv: []string{"GUSTO_SANDBOX=openshell", "GUSTO_OPENSHELL_GATEWAY=g"}}
	for _, each := range []struct {
		slug, sandbox, gateway string
	}{{signup.Slug, "container", ""}, {demo.Slug, "openshell", "g"}} {
		env, err := c.environment(each.slug, Runtime{Port: 1, ProxyPort: 2}, c.Cell)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := settingOf(env, "GUSTO_SANDBOX"); got != each.sandbox {
			t.Errorf("%s runs its agents in %q, not %q", each.slug, got, each.sandbox)
		}
		if got, _ := settingOf(env, "GUSTO_OPENSHELL_GATEWAY"); got != each.gateway {
			t.Errorf("%s has the gateway %q", each.slug, got)
		}
		if got, _ := settingOf(env, "GUSTO_MODELS"); got != "m" {
			t.Errorf("%s lost the operator's other settings: %q", each.slug, got)
		}
	}
	if len(c.Env) != 2 {
		t.Errorf("a demo's settings were written into the operator's: %v", c.Env)
	}
}
