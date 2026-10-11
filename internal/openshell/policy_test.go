package openshell

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func decoded(t *testing.T, cfg Config, model string, writable ...string) map[string]any {
	t.Helper()
	text, err := Policy(cfg, model, writable)
	if err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	if err := json.Unmarshal(text, &policy); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	return policy
}

func TestASandboxMayReachTheProxyAndTheHostsTheOperatorNamedAndNothingElse(t *testing.T) {
	policy := decoded(t, Config{Allowed: []string{"registry.npmjs.org", "api.example.com:8443", "*.pypi.org"}},
		"http://host.openshell.internal:8041/model/v1")
	rules := policy["network_policies"].(map[string]any)
	if len(rules) != 4 {
		t.Fatalf("the rules are %v", rules)
	}
	reach := map[string][]int{}
	for _, each := range rules {
		for _, e := range each.(map[string]any)["endpoints"].([]any) {
			endpoint := e.(map[string]any)
			for _, port := range endpoint["ports"].([]any) {
				reach[endpoint["host"].(string)] = append(reach[endpoint["host"].(string)], int(port.(float64)))
			}
		}
	}
	for host, want := range map[string][]int{
		"host.openshell.internal": {8041}, "registry.npmjs.org": {80, 443}, "api.example.com": {8443}, "*.pypi.org": {80, 443},
	} {
		if len(reach[host]) != len(want) || reach[host][0] != want[0] {
			t.Errorf("%s reaches %v, want %v", host, reach[host], want)
		}
	}
	if len(reach) != 4 {
		t.Errorf("the sandbox may reach more than was named: %v", reach)
	}
}

func TestTheWorkspaceAndTheHarnessesOwnPlacesAreWritableAndTheSystemIsNot(t *testing.T) {
	policy := decoded(t, Config{}, "http://host.openshell.internal:8041/model/v1", "/home/agent/.local/share/opencode")
	filesystem := policy["filesystem_policy"].(map[string]any)
	writable := map[string]bool{}
	for _, path := range filesystem["read_write"].([]any) {
		writable[path.(string)] = true
	}
	for _, want := range []string{"/sandbox", "/tmp", "/dev/null", "/dev/ptmx", "/dev/pts", "/home/agent/.local/share/opencode"} {
		if !writable[want] {
			t.Errorf("%s is not writable", want)
		}
	}
	for _, never := range []string{"/", "/etc", "/usr", "/root"} {
		if writable[never] {
			t.Errorf("%s is writable", never)
		}
	}
	if policy["landlock"].(map[string]any)["compatibility"] != "hard_requirement" {
		t.Error("a sandbox that cannot enforce the file rules would start without them")
	}
	if filesystem["include_workdir"] != false {
		t.Error("the working directory is added implicitly")
	}
}

func TestAnEntryThatCannotBeTranslatedIsRefusedAndNotDropped(t *testing.T) {
	for _, entry := range []string{"api.example.com:not-a-port", "*.com", ":443"} {
		if _, err := Policy(Config{Allowed: []string{entry}}, "http://host.openshell.internal:8041/model/v1", nil); err == nil {
			t.Errorf("%q was accepted", entry)
		}
	}
	for _, model := range []string{"", "not a url", "http://host.openshell.internal/model/v1x:notaport"} {
		if _, err := Policy(Config{}, model, nil); err == nil && !strings.Contains(model, "/model/v1x") {
			t.Errorf("%q was accepted as the proxy's address", model)
		}
	}
}

// A policy that names no system path leaves a sandbox unable to start its own
// supervisor, which was found on a real gateway, so every policy names them.
func TestEveryPolicyNamesTheSystemPathsASandboxNeedsToStart(t *testing.T) {
	denied, err := denying()
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string][]byte{"the survey's": denied, "a job's": mustPolicy(t)} {
		var policy map[string]any
		if err := json.Unmarshal(text, &policy); err != nil {
			t.Fatal(err)
		}
		readable := map[string]bool{}
		for _, path := range policy["filesystem_policy"].(map[string]any)["read_only"].([]any) {
			readable[path.(string)] = true
		}
		for _, need := range []string{"/usr", "/lib", "/bin", "/proc", "/etc/passwd", "/etc/group", "/etc/resolv.conf", "/etc/ld.so.cache"} {
			if !readable[need] {
				t.Errorf("%s policy does not let a sandbox read %s", name, need)
			}
		}
		if rules, _ := policy["network_policies"].(map[string]any); name == "the survey's" && len(rules) != 0 {
			t.Errorf("the survey sandbox may reach %v", rules)
		}
	}
}

func mustPolicy(t *testing.T) []byte {
	t.Helper()
	text, err := Policy(Config{}, "http://host.openshell.internal:8041/model/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestASandboxReadsOnlyWhatAProgramNeedsOfEtcAndNoLogs(t *testing.T) {
	policy := decoded(t, Config{}, "http://host.openshell.internal:8041/model/v1")
	readable := map[string]bool{}
	for _, path := range policy["filesystem_policy"].(map[string]any)["read_only"].([]any) {
		readable[path.(string)] = true
	}
	for _, never := range []string{"/etc", "/etc/shadow", "/etc/gshadow", "/var/log", "/var", "/root"} {
		if readable[never] {
			t.Errorf("%s is readable", never)
		}
	}
	for _, want := range []string{"/etc/passwd", "/etc/resolv.conf", "/etc/ssl", "/etc/ld.so.cache", "/usr", "/proc"} {
		if !readable[want] {
			t.Errorf("%s is not readable, and a harness needs it", want)
		}
	}
}

func TestEveryCommandInASandboxStartsUnderAProcessLimit(t *testing.T) {
	s := &Sandbox{cfg: Config{Binary: "openshell", Processes: 77}, name: "haas-x"}
	wrapped := s.Wrap(t.Context(), exec.Command("harness", "--go"))
	script := strings.Join(wrapped.Args, " ")
	if !strings.Contains(script, "ulimit -u 77") || !strings.Contains(script, "ulimit -p 77") {
		t.Errorf("the command sets no process limit: %s", script)
	}
	if strings.Index(script, "ulimit") > strings.Index(script, "exec \"$@\"") {
		t.Error("the limit is set after the harness starts")
	}
}

func TestAProcessLimitThatIsNotANumberAboveZeroIsRefused(t *testing.T) {
	for _, bad := range []string{"0", "-3", "many"} {
		t.Setenv("HAAS_OPENSHELL_PROCESSES", bad)
		if _, err := FromEnv(); err == nil {
			t.Errorf("%q was taken as a process limit", bad)
		}
	}
	t.Setenv("HAAS_OPENSHELL_PROCESSES", "")
	if cfg, err := FromEnv(); err != nil || cfg.Processes != 512 {
		t.Errorf("with none set the limit is %d, %v", cfg.Processes, err)
	}
}
