package main

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/settings"
)

func TestSubmitSendsTheTokenWhenThereIsOneAndThePasswordOtherwise(t *testing.T) {
	t.Parallel()
	for name, env := range map[string]struct{ token, password, want string }{
		"a token and a password": {"gusto_ab12cd34_secret", "shared", "gusto_ab12cd34_secret"},
		"a password alone":       {"", "shared", "shared"},
		"a token alone":          {"gusto_ab12cd34_secret", "", "gusto_ab12cd34_secret"},
		"neither":                {"", "", ""},
	} {
		s := submitSettings{token: env.token, password: env.password}
		if got := s.credential(); got != env.want {
			t.Errorf("%s: sent %q, want %q", name, got, env.want)
		}
	}
}

func TestSubmitReadsItsServerAndSignInFromTheSettingsItDeclares(t *testing.T) {
	t.Parallel()
	var s submitSettings
	set := settings.New()
	s.declare(set)
	given := map[string]string{"GUSTO_SERVER": "https://gusto.example.com", "GUSTO_TOKEN": "gusto_ab12cd34_secret"}
	if err := set.Load(func(name string) string { return given[name] }); err != nil {
		t.Fatal(err)
	}
	if s.server != "https://gusto.example.com" || s.credential() != "gusto_ab12cd34_secret" {
		t.Errorf("read %+v", s)
	}
	var help []string
	for _, each := range set.Params() {
		help = append(help, each.Name)
	}
	if got := strings.Join(help, " "); got != "GUSTO_SERVER GUSTO_TOKEN GUSTO_PASSWORD" {
		t.Errorf("help lists %s", got)
	}
}
