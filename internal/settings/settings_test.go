package settings

import (
	"strings"
	"testing"
)

type example struct {
	addr     string
	password string
	workers  int
	local    bool
	sandbox  string
	allowed  []string
}

func declareExample(set *Set, e *example) {
	set.String(&e.addr, "GUSTO_ADDR", "127.0.0.1:8040", "where to listen")
	set.Secret(&e.password, "GUSTO_PASSWORD", "the shared sign-in")
	set.Int(&e.workers, "GUSTO_JOB_CONCURRENCY", 1, 1, "how many tasks run at a time")
	set.Bool(&e.local, "GUSTO_LOCAL", "a server for one person on their own computer")
	set.Choice(&e.sandbox, "GUSTO_SANDBOX", "container", []string{"container", "openshell", "process", "here"}, "how tasks are held")
	set.List(&e.allowed, "GUSTO_ALLOWED", "who may sign in")
}

func load(t *testing.T, values map[string]string) (example, error) {
	t.Helper()
	var e example
	set := New()
	declareExample(set, &e)
	return e, set.Load(func(name string) string { return values[name] })
}

// A setting nobody set is its default, and one that was set is read as its type.
func TestSettingsAreTheirDefaultsUntilSetAndTheirTypeOnceSet(t *testing.T) {
	e, err := load(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e.addr != "127.0.0.1:8040" || e.workers != 1 || e.local || e.sandbox != "container" || e.allowed != nil {
		t.Errorf("the defaults are %+v", e)
	}
	e, err = load(t, map[string]string{"GUSTO_ADDR": "0.0.0.0:9", "GUSTO_PASSWORD": "s", "GUSTO_JOB_CONCURRENCY": "3",
		"GUSTO_LOCAL": "1", "GUSTO_SANDBOX": "here", "GUSTO_ALLOWED": " ada@acme.com , ,bob@acme.com,"})
	if err != nil {
		t.Fatal(err)
	}
	if e.addr != "0.0.0.0:9" || e.password != "s" || e.workers != 3 || !e.local || e.sandbox != "here" ||
		strings.Join(e.allowed, "|") != "ada@acme.com|bob@acme.com" {
		t.Errorf("the settings read as %+v", e)
	}
}

// Every value that does not parse is said by name in one error, and a secret's value is never in it.
func TestAWrongValueStopsTheCommandAndSaysWhich(t *testing.T) {
	_, err := load(t, map[string]string{"GUSTO_JOB_CONCURRENCY": "two", "GUSTO_LOCAL": "maybe", "GUSTO_SANDBOX": "docker"})
	if err == nil {
		t.Fatal("wrong values were read")
	}
	for _, want := range []string{`GUSTO_JOB_CONCURRENCY is "two"`, `GUSTO_LOCAL is "maybe"`, `GUSTO_SANDBOX is "docker", and it is one of container`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q is not in: %v", want, err)
		}
	}
	if _, err := load(t, map[string]string{"GUSTO_JOB_CONCURRENCY": "0"}); err == nil || !strings.Contains(err.Error(), "1 or more") {
		t.Errorf("a number below its least was read: %v", err)
	}
	var secret int
	set := New()
	set.Int(&secret, "GUSTO_SECRET_NUMBER", 0, 0, "a number")
	set.entries[0].secret = true
	if err := set.Load(func(string) string { return "sk-not-a-number" }); err == nil || strings.Contains(err.Error(), "sk-not-a-number") {
		t.Errorf("a secret's value was said: %v", err)
	}
}

// Help lists every setting with its default, and a record of what was set leaves a secret out.
func TestHelpListsEverySettingAndNoSecretIsShown(t *testing.T) {
	var e example
	set := New()
	declareExample(set, &e)
	if err := set.Load(func(name string) string {
		return map[string]string{"GUSTO_PASSWORD": "s3cret", "GUSTO_ADDR": "0.0.0.0:9"}[name]
	}); err != nil {
		t.Fatal(err)
	}
	params := set.Params()
	if len(params) != 6 || params[0].Text != "where to listen (default 127.0.0.1:8040)" {
		t.Errorf("help is %+v", params)
	}
	for _, p := range params {
		if strings.Contains(p.Text, "s3cret") {
			t.Errorf("help shows a secret: %+v", p)
		}
	}
	if set.Raw("GUSTO_PASSWORD") != "" || set.Raw("GUSTO_ADDR") != "0.0.0.0:9" {
		t.Error("the record of what was set is wrong")
	}
}

// A default the help already says is not said again.
func TestHelpSaysADefaultOnce(t *testing.T) {
	var limit int
	set := New()
	set.Int(&limit, "GUSTO_LIMIT", 0, 0, "the most at once (default no limit)")
	if got := set.Params()[0].Text; got != "the most at once (default no limit)" {
		t.Errorf("help is %q", got)
	}
}

// A setting declared twice, or with nothing to say what it does, is a mistake in the code, found the moment it is declared.
func TestASettingDeclaredTwiceOrWithNoHelpPanics(t *testing.T) {
	for name, declare := range map[string]func(*Set){
		"twice":   func(s *Set) { var a, b string; s.String(&a, "GUSTO_X", "", "x"); s.String(&b, "GUSTO_X", "", "x") },
		"no help": func(s *Set) { var a string; s.String(&a, "GUSTO_Y", "", "") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("a setting declared %s did not panic", name)
				}
			}()
			declare(New())
		}()
	}
}

// What a command decides itself is read before what the environment says, and the rest comes from the environment.
func TestOverReadsWhatTheCommandDecidedFirst(t *testing.T) {
	t.Parallel()
	environment := map[string]string{"GUSTO_ADDR": "0.0.0.0:80", "GUSTO_ORG": "acme"}
	lookup := Over(map[string]string{"GUSTO_ADDR": "127.0.0.1:8040", "GUSTO_LOCAL": "1"}, func(name string) string { return environment[name] })
	for name, want := range map[string]string{"GUSTO_ADDR": "127.0.0.1:8040", "GUSTO_LOCAL": "1", "GUSTO_ORG": "acme", "GUSTO_NONE": ""} {
		if got := lookup(name); got != want {
			t.Errorf("%s is %q, want %q", name, got, want)
		}
	}
}
