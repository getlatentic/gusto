package main

import "testing"

func TestSubmitSendsTheTokenWhenThereIsOneAndThePasswordOtherwise(t *testing.T) {
	for name, env := range map[string]struct{ token, password, want string }{
		"a token and a password": {"haas_ab12cd34_secret", "shared", "haas_ab12cd34_secret"},
		"a password alone":       {"", "shared", "shared"},
		"a token alone":          {"haas_ab12cd34_secret", "", "haas_ab12cd34_secret"},
		"neither":                {"", "", ""},
	} {
		t.Setenv("HAAS_TOKEN", env.token)
		t.Setenv("HAAS_PASSWORD", env.password)
		if got := credential(); got != env.want {
			t.Errorf("%s: sent %q, want %q", name, got, env.want)
		}
	}
}
