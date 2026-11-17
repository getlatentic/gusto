package settings

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fromFile applies a settings file holding text over env, the way a command starts, and loads the example's settings from
// what results.
func fromFile(t *testing.T, text string, env map[string]string) (example, error) {
	t.Helper()
	var e example
	set := New()
	declareExample(set, &e)
	set.Renamed("GUSTO_TASK_CONCURRENCY", "GUSTO_WORKERS")
	path := filepath.Join(t.TempDir(), "gusto.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	values := maps.Clone(env)
	if values == nil {
		values = map[string]string{}
	}
	values[FileEnv] = path
	err := ApplyFile(func(name string) string { return values[name] }, func(name, value string) error {
		values[name] = value
		return nil
	})
	if err != nil {
		return e, err
	}
	return e, set.Load(func(name string) string { return values[name] })
}

// A setting's key in a file is its name without GUSTO_, in lower case.
func TestASettingsKeyIsItsNameWithoutGustoInLowerCase(t *testing.T) {
	for name, want := range map[string]string{"GUSTO_CLOUD_FREE_MAIL": "cloud_free_mail", "GUSTO_ADDR": "addr", "GUSTO_TASK_CPUS": "task_cpus"} {
		if got := Key(name); got != want || nameOf(got) != name {
			t.Errorf("Key(%q) = %q, and back %q", name, got, nameOf(got))
		}
	}
}

// Every kind of value a file holds is read as the setting's type, and a list may be a TOML list or one comma-separated string.
func TestAFilesValuesAreReadAsTheirSettingsType(t *testing.T) {
	e, err := fromFile(t, `addr = "0.0.0.0:9"
task_concurrency = 3
local = true
sandbox = "here"
allowed = ["ada@acme.com", "bob@acme.com"]
`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e.addr != "0.0.0.0:9" || e.workers != 3 || !e.local || e.sandbox != "here" || strings.Join(e.allowed, "|") != "ada@acme.com|bob@acme.com" {
		t.Errorf("the file read as %+v", e)
	}
	if e, err = fromFile(t, `allowed = "ada@acme.com, bob@acme.com"`, nil); err != nil || len(e.allowed) != 2 {
		t.Errorf("a comma-separated list read as %q: %v", e.allowed, err)
	}
}

// A setting in the environment is read over the same one in the file, and the file fills in what the environment leaves out.
func TestTheEnvironmentIsReadOverTheFile(t *testing.T) {
	e, err := fromFile(t, "addr = \"0.0.0.0:9\"\ntask_concurrency = 3\n", map[string]string{"GUSTO_ADDR": "127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if e.addr != "127.0.0.1:1" || e.workers != 3 {
		t.Errorf("read %q and %d", e.addr, e.workers)
	}
}

// What a file may not hold stops the command, by key: a secret, which only the environment carries; an old name; a key no
// command reads; a value of a kind no setting is; and a value that does not parse.
func TestAFileIsRefusedForWhatItMayNotHold(t *testing.T) {
	cases := map[string]struct{ text, want string }{
		"a secret":          {`password = "s"`, "password is a secret, which is read from the environment only, as GUSTO_PASSWORD"},
		"an unknown key":    {`adr = "0.0.0.0:9"`, "adr is not a setting"},
		"an old name":       {`workers = 2`, "workers is now called task_concurrency"},
		"a table":           {"[addr]\nport = 9", "addr is a string, a whole number, true or false, or a list of strings"},
		"a list of numbers": {`allowed = [1, 2]`, "allowed is a list of strings"},
		"a wrong value":     {`task_concurrency = 0`, `GUSTO_TASK_CONCURRENCY is "0", and it is 1 or more`},
		"not TOML":          {`addr = `, "could not be read"},
	}
	for name, c := range cases {
		if _, err := fromFile(t, c.text, nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, and wanted %q", name, err, c.want)
		}
	}
}

// A key another of the program's commands declares is right in a file this one reads too, since one file may serve them all.
func TestAKeyAnotherCommandDeclaresIsRight(t *testing.T) {
	var mail string
	New().String(&mail, "GUSTO_CLOUD_FREE_MAIL", "", "whether free mail may sign up")
	if _, err := fromFile(t, `cloud_free_mail = "allow"`, nil); err != nil {
		t.Error(err)
	}
}

// A file named and not there stops the command; no file named sets nothing.
func TestAFileNamedMustBeThere(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.toml")
	err := ApplyFile(func(name string) string { return map[string]string{FileEnv: missing}[name] }, nil)
	if err == nil || !strings.Contains(err.Error(), FileEnv) {
		t.Errorf("a missing file was read: %v", err)
	}
	if err := ApplyFile(func(string) string { return "" }, nil); err != nil {
		t.Error(err)
	}
}
