package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAServerIsAnOrganisationsWhenAllThreeSettingsAreGiven(t *testing.T) {
	key := filepath.Join(t.TempDir(), "entry.key")
	os.WriteFile(key, []byte(strings.Repeat("k", 32)), 0o600)
	t.Setenv("HAAS_ENTRY_KEY", key)
	t.Setenv("HAAS_ORG", "acme-com")
	t.Setenv("HAAS_FRONT_DOOR", "https://cloud.example.com/some/path")
	front, err := frontDoorFromEnv()
	if err != nil || front == nil || front.Org != "acme-com" || front.URL != "https://cloud.example.com" || len(front.Key) != 32 {
		t.Fatalf("%+v %v", front, err)
	}
}

func TestAServerWithNoneOfThemIsAnOrdinaryOne(t *testing.T) {
	front, err := frontDoorFromEnv()
	if err != nil || front != nil {
		t.Errorf("%+v %v", front, err)
	}
}

func TestSomeOfTheSettingsOrBadOnesRefuseToStart(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short.key")
	os.WriteFile(short, []byte("short"), 0o600)
	good := filepath.Join(dir, "good.key")
	os.WriteFile(good, []byte(strings.Repeat("k", 32)), 0o600)
	for name, env := range map[string]map[string]string{
		"only the org":               {"HAAS_ORG": "acme-com"},
		"no front door":              {"HAAS_ENTRY_KEY": good, "HAAS_ORG": "acme-com"},
		"a key that is gone":         {"HAAS_ENTRY_KEY": filepath.Join(dir, "gone"), "HAAS_ORG": "acme-com", "HAAS_FRONT_DOOR": "https://x.example.com"},
		"a short key":                {"HAAS_ENTRY_KEY": short, "HAAS_ORG": "acme-com", "HAAS_FRONT_DOOR": "https://x.example.com"},
		"an address that is not one": {"HAAS_ENTRY_KEY": good, "HAAS_ORG": "acme-com", "HAAS_FRONT_DOOR": "cloud.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, unset := range []string{"HAAS_ENTRY_KEY", "HAAS_ORG", "HAAS_FRONT_DOOR"} {
				t.Setenv(unset, "")
			}
			for key, value := range env {
				t.Setenv(key, value)
			}
			if front, err := frontDoorFromEnv(); err == nil {
				t.Errorf("%+v", front)
			}
		})
	}
}
