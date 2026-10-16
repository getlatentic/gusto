package doctor

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
)

// fakeRuntime answers the four things the checks ask a container runtime, each
// with the outcome a test chose, so no test needs a runtime on the machine.
type fakeRuntime struct {
	info, inspect, limits, mount Outcome
}

// answering is a runtime where everything works.
func answering() fakeRuntime {
	ok := Outcome{Started: true}
	return fakeRuntime{info: ok, inspect: ok, limits: ok, mount: ok}
}

func (f fakeRuntime) commands(program string, args ...string) Outcome {
	switch {
	case args[0] == "info":
		return f.info
	case args[0] == "image":
		return f.inspect
	case slices.Contains(args, "-v"):
		return f.mount
	default:
		return f.limits
	}
}

// refusingTinyMemory is docker's own answer to a cap under its floor, measured
// at GUSTO_JOB_MEMORY=4m.
func refusingTinyMemory(program string, args ...string) Outcome {
	if slices.Contains(args, "--memory=4m") {
		return Outcome{Started: true, Code: 125,
			Output: "docker: Error response from daemon: Minimum memory limit allowed is 6MB."}
	}
	return answering().commands(program, args...)
}

func machineWith(commands Commands) Machine {
	return Machine{
		Commands: commands,
		LookPath: func(program string) (string, error) { return "/usr/bin/" + program, nil },
		Writable: func(string) error { return nil },
		User:     func() int { return 1000 },
	}
}

// healthy sets the environment of a runner that has everything it needs.
func healthy(t *testing.T) {
	t.Helper()
	t.Setenv("GUSTO_JOBS", t.TempDir())
	t.Setenv("GUSTO_PASSWORD", "correct horse battery staple")
	for _, name := range []string{"GUSTO_CONTAINER", "GUSTO_IMAGE", "GUSTO_JOB_MEMORY", "GUSTO_JOB_CPUS",
		"GUSTO_MODEL_URL", "GUSTO_HARNESSES", "GUSTO_SANDBOX", "XDG_RUNTIME_DIR"} {
		t.Setenv(name, "")
	}
}

// examine runs every check against the machine and hands back what was printed.
func examine(machine Machine) (string, bool) {
	results := Run(Checks(machine))
	var printed bytes.Buffer
	Print(&printed, results)
	return printed.String(), Healthy(results)
}

// failed holds that a check's line says fail, and that the line under it says
// what to do.
func failed(t *testing.T, printed, name, remedy string) {
	t.Helper()
	lines := strings.Split(printed, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "fail") && strings.Contains(line, " "+name+" ") {
			if i+1 < len(lines) && strings.Contains(lines[i+1], "fix:") && strings.Contains(lines[i+1], remedy) {
				return
			}
			t.Fatalf("the %s failure should say %q on the next line, got:\n%s", name, remedy, printed)
		}
	}
	t.Fatalf("the output should name %s as failing, got:\n%s", name, printed)
}

func status(t *testing.T, printed, name string, want Status) {
	t.Helper()
	for _, line := range strings.Split(printed, "\n") {
		if strings.Contains(line, " "+name+" ") && !strings.Contains(line, "fix:") {
			if !strings.HasPrefix(line, string(want)) {
				t.Fatalf("%s should be %s, got %q", name, want, line)
			}
			return
		}
	}
	t.Fatalf("no line for %s in:\n%s", name, printed)
}

var errDenied = errors.New("permission denied")
