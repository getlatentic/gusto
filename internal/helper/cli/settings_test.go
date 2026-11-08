package cli

import (
	"testing"

	"github.com/getlatentic/gusto/internal/settings"
)

func lookupIn(given map[string]string) func(string) string {
	return func(name string) string { return given[name] }
}

func TestTheHelperReadsItsSettingsFromWhatItDeclares(t *testing.T) {
	t.Parallel()
	var s acpSettings
	set := settings.New()
	s.declare(set)
	err := set.Load(lookupIn(map[string]string{
		"GUSTO_SERVER": "https://gusto.example.com", "GUSTO_VAULT": "file", "GUSTO_INSTANCE": "mine", "GUSTO_AGENT_KIND": "codex",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if s.server != "https://gusto.example.com" || s.vault != "file" || s.instance != "mine" || s.kind != "codex" {
		t.Errorf("read %+v", s)
	}
}

func TestTheTokenIsKeptInTheKeychainUnlessTheFileIsAskedFor(t *testing.T) {
	t.Parallel()
	var s account
	set := settings.New()
	s.declare(set)
	if err := set.Load(lookupIn(nil)); err != nil || s.vault != "keychain" {
		t.Errorf("with nothing set the vault is %q, %v", s.vault, err)
	}
}

func TestAVaultTheHelperDoesNotKnowStopsTheCommand(t *testing.T) {
	t.Parallel()
	var s account
	if loaded(lookupIn(map[string]string{"GUSTO_VAULT": "files"}), s.declare) {
		t.Error("GUSTO_VAULT=files was taken, and the token would have gone to the keychain")
	}
}

func TestEachCommandDeclaresTheSettingsItReads(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		declare func(*settings.Set)
		want    []string
	}{
		"account": {AccountSettings, []string{"GUSTO_SERVER", "GUSTO_VAULT"}},
		"service": {ServiceSettings, []string{"GUSTO_SERVER"}},
		"sync":    {SyncSettings, []string{"GUSTO_SERVER", "GUSTO_VAULT", "GUSTO_INSTANCE"}},
		"acp":     {ACPSettings, []string{"GUSTO_SERVER", "GUSTO_VAULT", "GUSTO_INSTANCE", "GUSTO_AGENT_KIND"}},
	} {
		set := settings.New()
		c.declare(set)
		params := set.Params()
		if len(params) != len(c.want) {
			t.Errorf("%s declares %v, want %v", name, params, c.want)
			continue
		}
		for i, each := range params {
			if each.Name != c.want[i] || each.Text == "" {
				t.Errorf("%s: setting %d is %+v, want %s with help", name, i, each, c.want[i])
			}
		}
	}
}
