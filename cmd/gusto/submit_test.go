package main

import "testing"

func TestSubmitSendsTheTokenWhenThereIsOneAndThePasswordOtherwise(t *testing.T) {
	for name, env := range map[string]struct{ token, password, want string }{
		"a token and a password": {"gusto_ab12cd34_secret", "shared", "gusto_ab12cd34_secret"},
		"a password alone":       {"", "shared", "shared"},
		"a token alone":          {"gusto_ab12cd34_secret", "", "gusto_ab12cd34_secret"},
		"neither":                {"", "", ""},
	} {
		t.Setenv("GUSTO_TOKEN", env.token)
		t.Setenv("GUSTO_PASSWORD", env.password)
		if got := credential(); got != env.want {
			t.Errorf("%s: sent %q, want %q", name, got, env.want)
		}
	}
}
