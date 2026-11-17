package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/settings"
)

// helpNames is every setting gusto's help lists, which is what a settings file is read against when a command starts.
func helpNames() map[string]bool {
	names := map[string]bool{}
	var walk func([]*cmdline.Command)
	walk = func(commands []*cmdline.Command) {
		for _, command := range commands {
			for _, each := range command.Env {
				names[each.Name] = true
			}
			walk(command.Subs)
		}
	}
	walk(program().Commands)
	return names
}

// A command reads every setting it loads from the settings file, so help must list each, or the file's key for it is refused
// before the command runs.
func TestEverySettingACommandLoadsIsOneHelpLists(t *testing.T) {
	listed := helpNames()
	for name, declare := range map[string]func(*settings.Set){
		"serve": new(serveSettings).declare, "cloud": new(cloudSettings).declare, "doctor": new(doctorSettings).declare,
		"sandbox": new(sandboxSettings).declare, "node": new(nodeSettings).declare, "operator": new(operatorSettings).declare,
	} {
		set := settings.New()
		declare(set)
		for _, each := range set.Params() {
			if !listed[each.Name] {
				t.Errorf("%s loads %s, which no command's help lists", name, each.Name)
			}
		}
	}
}

// gusto cloud reads a setting from the settings file as from the environment, and the environment over the file.
func TestGustoCloudReadsItsSettingsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cloud.toml")
	if err := os.WriteFile(path, []byte("cloud_orgs_at = \"host\"\ncloud_url = \"https://cloud.example.com\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(settings.FileEnv, path)
	t.Setenv("GUSTO_CLOUD_URL", "https://other.example.com")
	program()
	if err := settings.ApplyFile(os.Getenv, func(name, value string) error { t.Setenv(name, value); return nil }); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadCloud(func(name string) string {
		if name == "GUSTO_CLOUD_SERVERS" {
			return "off"
		}
		return os.Getenv(name)
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.front.orgsAt != "host" || cfg.front.url != "https://other.example.com" {
		t.Errorf("organizations are at %q and gusto cloud at %q", cfg.front.orgsAt, cfg.front.url)
	}
}
