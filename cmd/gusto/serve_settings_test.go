package main

import (
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/settings"
)

// Every setting serve reads is declared once, with help: declaring one twice or with nothing to say panics, and help lists them all.
func TestEverySettingServeReadsIsDeclaredOnceWithHelp(t *testing.T) {
	t.Parallel()
	var cfg serveSettings
	set := settings.New()
	cfg.declare(set)
	params := set.Params()
	if len(params) < 90 {
		t.Errorf("serve declares %d settings", len(params))
	}
	for _, p := range params {
		if !strings.HasPrefix(p.Name, "GUSTO_") || p.Text == "" {
			t.Errorf("%+v", p)
		}
	}
}

// Serve's settings read as their types, and a value that does not read is said by name rather than left at its default.
func TestServesSettingsReadAsTheirTypesAndAWrongOneIsSaid(t *testing.T) {
	t.Parallel()
	read := func(values map[string]string) (serveSettings, error) {
		var cfg serveSettings
		set := settings.New()
		cfg.declare(set)
		return cfg, set.Load(func(name string) string { return values[name] })
	}
	cfg, err := read(map[string]string{"GUSTO_JOB_CONCURRENCY": "3", "GUSTO_SANDBOX": "here", "GUSTO_LOCAL": "1", "GUSTO_SPARE_COMPUTER": "on"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.boxes.concurrency != 3 || !cfg.boxes.here() || !cfg.core.local || !cfg.boxes.spare || cfg.core.addr != "127.0.0.1:8040" {
		t.Errorf("the settings read as %+v", cfg)
	}
	_, err = read(map[string]string{"GUSTO_JOB_CONCURRENCY": "two", "GUSTO_SANDBOX": "docker", "GUSTO_OIDC_CLIENT_SECRET": "x", "GUSTO_KEYCHAIN": "maybe"})
	for _, want := range []string{"GUSTO_JOB_CONCURRENCY", "GUSTO_SANDBOX", "GUSTO_KEYCHAIN"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s is not said in %v", want, err)
		}
	}
}

// A server with a setting that does not read does not start, as one with no password does not.
func TestAServerWithAWrongSettingDoesNotStart(t *testing.T) {
	t.Setenv("GUSTO_PASSWORD", "p")
	t.Setenv("GUSTO_JOB_CONCURRENCY", "two")
	if s, code := newServer(os.Getenv); s != nil || code != 2 {
		t.Errorf("a server with a wrong setting started: %v %d", s, code)
	}
}
