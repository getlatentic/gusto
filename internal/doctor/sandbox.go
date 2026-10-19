package doctor

import (
	"os"
	"runtime"
	"strings"

	"github.com/getlatentic/gusto/internal/isolation"
)

// sandboxChoice says which wall gusto would put between an agent and this computer's files, in words, and why a stronger one was not
// chosen. Having none is not a failure of the computer, so it passes, with what would give it one.
func sandboxChoice(machine Machine) Verdict {
	host := isolation.Host{
		GOOS:     runtime.GOOS,
		Getenv:   os.Getenv,
		Exists:   func(path string) bool { _, err := os.Stat(path); return err == nil },
		LookPath: machine.LookPath,
		Run: func(program string, args ...string) (string, bool) {
			outcome := machine.Commands(program, args...)
			return strings.TrimSpace(outcome.Output), outcome.Started && outcome.Code == 0
		},
	}
	host.MountsSocket = isolation.SocketMount(host.Run)
	choice := isolation.Choose(host)
	verdict := Verdict{Status: Pass, Said: choice.Words()}
	if choice.Forced {
		verdict.Said += " (set by GUSTO_SANDBOX)"
	}
	var missing []string
	for _, backend := range choice.Passed {
		said := string(backend.Kind) + ": " + backend.Detail
		if backend.Cause != "" {
			said += " (" + backend.Cause + ")"
		}
		missing = append(missing, said)
	}
	if len(missing) > 0 {
		verdict.Remedy = "no stronger sandbox: " + strings.Join(missing, "; ")
	}
	return verdict
}
