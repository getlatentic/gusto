package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/openshell"
	"github.com/getlatentic/haas/internal/runner"
)

// modelAnswers dials the model the way a job would, before the harness starts.
// Only a model named in HAAS_MODEL_URL is checked: without it a job that needs a
// local model finds out at its own start, and a job that needs none never asks.
func modelAnswers() Verdict {
	endpoint := os.Getenv("HAAS_MODEL_URL")
	if endpoint == "" {
		return skip("HAAS_MODEL_URL is not set")
	}
	if err := runner.Reachable(endpoint); err != nil {
		return fail(err.Error(),
			"start the model, and have it listen on more than loopback so a container reaches it (for Ollama, OLLAMA_HOST=0.0.0.0)")
	}
	return pass("a model answers at " + endpoint)
}

// adaptersParse reads every adapter of your own. One that does not parse is
// otherwise found by the first job that names it, or by the pages offering a
// harness the server cannot run.
func adaptersParse() Verdict {
	dir := os.Getenv("HAAS_HARNESSES")
	if dir == "" {
		return skip("HAAS_HARNESSES is not set, so only the built-in adapters are used")
	}
	names, err := adapterNames(dir)
	if err != nil {
		return fail(err.Error(), "set HAAS_HARNESSES to a directory of adapter .json files")
	}
	if broken := unparsed(names); len(broken) > 0 {
		return fail("these adapters do not parse: "+strings.Join(broken, "; "),
			"correct them against an adapter in the README, which names at least a program")
	}
	return pass(fmt.Sprintf("%d adapter(s) in %s parse", len(names), dir))
}

func adapterNames(dir string) ([]string, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s holds no adapter files", dir)
	}
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = strings.TrimSuffix(filepath.Base(file), ".json")
	}
	return names, nil
}

func unparsed(names []string) []string {
	var broken []string
	for _, name := range names {
		adapter, err := harness.Load("", name)
		switch {
		case err != nil:
			broken = append(broken, name+": "+err.Error())
		case adapter.Program == "":
			broken = append(broken, name+": no program")
		}
	}
	return broken
}

// signIn says whether haas serve has a password, and never what it is: this
// output goes to terminals, logs and pull requests.
func signIn() Verdict {
	if os.Getenv("HAAS_PASSWORD") == "" {
		return fail("HAAS_PASSWORD is not set, and haas serve will not start without it",
			"set HAAS_PASSWORD in the environment haas serve starts with")
	}
	return pass("HAAS_PASSWORD is set")
}

// sandboxHardware is whether this machine can be where an OpenShell gateway makes microVMs. A deployment that runs jobs
// in containers needs none, so the check is skipped for it.
func sandboxHardware() Verdict {
	if os.Getenv("HAAS_SANDBOX") != "openshell" {
		return skip("HAAS_SANDBOX is not openshell")
	}
	if err := openshell.HostCannotMakeVMs(); err != nil {
		return fail(err.Error(), "run on a host with KVM, or set HAAS_OPENSHELL_REMOTE=1 when the gateway is on another machine")
	}
	return pass("this machine can host the gateway's microVMs, or the gateway is elsewhere")
}
