package orgsettings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnOrganizationStartsWithNoSettingsAndWhatIsChangedIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	file := Open(path)
	if got, err := file.Read(); err != nil || got.MonthlyBudgetUSD != 0 {
		t.Fatalf("with no file: %+v %v", got, err)
	}
	if err := file.Change(func(s *Settings) { s.MonthlyBudgetUSD = 250.5 }); err != nil {
		t.Fatal(err)
	}
	if got, err := Open(path).Read(); err != nil || got.MonthlyBudgetUSD != 250.5 {
		t.Errorf("another reader: %+v %v", got, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("the settings are private: %v", info.Mode())
	}
}

func TestSettingsThatCannotBeReadAreAnErrorAndAreNotWrittenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	file := Open(path)
	for name, content := range map[string]string{
		"not json":         "{",
		"a budget below 0": `{"monthlyBudgetUsd": -5}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := file.Read(); err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("%s was read: %v", name, err)
		}
		if err := file.Change(func(s *Settings) { s.MonthlyBudgetUSD = 1 }); err == nil {
			t.Errorf("%s was written over", name)
		}
		if text, _ := os.ReadFile(path); string(text) != content {
			t.Errorf("%s: the file was changed: %q", name, text)
		}
	}
}

func TestAMembersBudgetIsTheirOwnElseEveryones(t *testing.T) {
	settings := Settings{MemberBudgetUSD: 20, MemberBudgets: map[string]float64{"ada@acme.com": 50, "bob@acme.com": 0}}
	for who, want := range map[string]struct {
		usd float64
		set bool
	}{"Ada@Acme.com": {50, true}, "carol@acme.com": {20, true}, "bob@acme.com": {0, false}} {
		if usd, set := settings.MemberBudget(who); usd != want.usd || set != want.set {
			t.Errorf("%s: %v %v", who, usd, set)
		}
	}
	if _, set := (Settings{}).MemberBudget("ada@acme.com"); set {
		t.Error("with no budgets a member has one")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"memberBudgets":{"ada@acme.com":-1}}`), 0o644)
	if _, err := Open(path).Read(); err == nil {
		t.Error("a budget below nothing was read")
	}
}
