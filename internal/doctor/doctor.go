// Package doctor checks what a runner needs from its machine before a task.
//
// A runner that cannot start a container finds out at the first task, and on a
// deployed box that means a whole deploy cycle. Rootless podman that could not
// reach its runtime directory failed every task with exit 125 before it began,
// and nothing said so until a task was sent. Each check here is one of those
// failures, named, with what to do about it.
//
// A check is a name and a function. Running them is separate from the checks,
// and printing the results is separate from both.
package doctor

// Status is what one check found.
type Status string

const (
	Pass    Status = "pass"
	Fail    Status = "fail"
	Skipped Status = "skip"
)

// Verdict is a check's answer: what it found, and on a fail what to do about it.
type Verdict struct {
	Status Status
	Said   string
	Remedy string
}

// Check is one thing a runner needs. Needs names the checks that must pass
// first, because an image cannot be inspected by a runtime that does not answer,
// and a second failure there would only repeat the first.
type Check struct {
	Name  string
	Needs []string
	Try   func() Verdict
}

// Result is a check's verdict under its name.
type Result struct {
	Name string
	Verdict
}

// Run tries every check in order and hands back what each found.
func Run(checks []Check) []Result {
	passed := map[string]bool{}
	results := make([]Result, 0, len(checks))
	for _, check := range checks {
		verdict := attempt(check, passed)
		passed[check.Name] = verdict.Status == Pass
		results = append(results, Result{Name: check.Name, Verdict: verdict})
	}
	return results
}

// Healthy is true when nothing failed. A skipped check is one that does not
// apply, so it does not make a machine unhealthy on its own; one skipped because
// something it needs failed sits beside that failure.
func Healthy(results []Result) bool {
	for _, result := range results {
		if result.Status == Fail {
			return false
		}
	}
	return true
}

func attempt(check Check, passed map[string]bool) Verdict {
	for _, need := range check.Needs {
		if !passed[need] {
			return skip("the " + need + " check did not pass")
		}
	}
	return check.Try()
}

func pass(said string) Verdict { return Verdict{Status: Pass, Said: said} }

func skip(why string) Verdict { return Verdict{Status: Skipped, Said: why} }

func fail(said, remedy string) Verdict {
	return Verdict{Status: Fail, Said: said, Remedy: remedy}
}
