package doctor

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/sandbox"
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
// at GUSTO_TASK_MEMORY=4m.
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

// healthy is the settings of a runner that has everything it needs.
func healthy(t *testing.T) Settings {
	t.Helper()
	return Settings{
		Containers: sandbox.Containers{Runtime: "docker", Image: "gusto:dev", CPUs: "1", Memory: "3g"},
		Tasks:      t.TempDir(),
		Password:   true,
	}
}

// examine runs every check against the machine set as s and hands back what was printed.
func examine(machine Machine, s Settings) (string, bool) {
	results := Run(Checks(machine, s))
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
