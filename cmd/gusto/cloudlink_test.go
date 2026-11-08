package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAServerIsAnOrganizationsWhenAllThreeSettingsAreGiven(t *testing.T) {
	t.Parallel()
	key := filepath.Join(t.TempDir(), "entry.key")
	os.WriteFile(key, []byte(strings.Repeat("k", 32)), 0o600)
	front, err := cloudOf(accessSettings{entryKey: key, cloudURL: "https://cloud.example.com/some/path"}, "acme-com")
	if err != nil || front == nil || front.Org != "acme-com" || front.URL != "https://cloud.example.com" || len(front.Key) != 32 {
		t.Fatalf("%+v %v", front, err)
	}
}

func TestAServerWithNoneOfThemIsAnOrdinaryOne(t *testing.T) {
	t.Parallel()
	front, err := cloudOf(accessSettings{}, "")
	if err != nil || front != nil {
		t.Errorf("%+v %v", front, err)
	}
}

func TestSomeOfTheSettingsOrBadOnesRefuseToStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	short := filepath.Join(dir, "short.key")
	os.WriteFile(short, []byte("short"), 0o600)
	good := filepath.Join(dir, "good.key")
	os.WriteFile(good, []byte(strings.Repeat("k", 32)), 0o600)
	for name, each := range map[string]struct {
		access accessSettings
		org    string
	}{
		"only the org":               {accessSettings{}, "acme-com"},
		"no gusto cloud":             {accessSettings{entryKey: good}, "acme-com"},
		"a key that is gone":         {accessSettings{entryKey: filepath.Join(dir, "gone"), cloudURL: "https://x.example.com"}, "acme-com"},
		"a short key":                {accessSettings{entryKey: short, cloudURL: "https://x.example.com"}, "acme-com"},
		"an address that is not one": {accessSettings{entryKey: good, cloudURL: "cloud.example.com"}, "acme-com"},
	} {
		if front, err := cloudOf(each.access, each.org); err == nil {
			t.Errorf("%s: %+v", name, front)
		}
	}
}
