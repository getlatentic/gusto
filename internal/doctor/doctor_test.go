package doctor

import (
	"strings"
	"testing"
)

func TestAMachineWithEverythingPasses(t *testing.T) {
	healthy(t)
	printed, ok := examine(machineWith(answering().commands))
	if !ok {
		t.Fatalf("a healthy machine should pass, got:\n%s", printed)
	}
	if strings.Contains(printed, "fail") {
		t.Fatalf("nothing should fail, got:\n%s", printed)
	}
}

// A check that cannot run because something before it failed says so, rather
// than failing a second time and burying the first cause.
func TestACheckWhoseNeedFailedIsSkippedAndSaysWhy(t *testing.T) {
	results := Run([]Check{
		{Name: "first", Try: func() Verdict { return fail("broken", "mend it") }},
		{Name: "second", Needs: []string{"first"}, Try: func() Verdict {
			t.Fatal("a check whose need failed must not run")
			return Verdict{}
		}},
	})
	if results[1].Status != Skipped || !strings.Contains(results[1].Said, "first") {
		t.Fatalf("the second check should be skipped naming the first, got %+v", results[1])
	}
	if Healthy(results) {
		t.Fatal("a failure makes the machine unhealthy")
	}
}

func TestSkippedChecksAloneLeaveAMachineHealthy(t *testing.T) {
	if !Healthy([]Result{{Name: "model", Verdict: skip("GUSTO_MODEL_URL is not set")}}) {
		t.Fatal("a check that does not apply is not a failure")
	}
}

// The output of doctor lands in terminals, deploy logs and bug reports, so the
// password is never in it, whether the rest of the machine passes or not.
func TestThePasswordIsNeverPrinted(t *testing.T) {
	healthy(t)
	secret := "correct horse battery staple"
	for _, commands := range []Commands{answering().commands, fakeRuntime{}.commands} {
		printed, _ := examine(machineWith(commands))
		if strings.Contains(printed, secret) {
			t.Fatalf("the password was printed:\n%s", printed)
		}
		status(t, printed, "sign-in", Pass)
	}
}

func TestAMissingPasswordIsNamedForServe(t *testing.T) {
	healthy(t)
	t.Setenv("GUSTO_PASSWORD", "")
	printed, ok := examine(machineWith(answering().commands))
	if ok {
		t.Fatal("serve will not start without a password")
	}
	failed(t, printed, "sign-in", "set GUSTO_PASSWORD")
}

func TestJobsRunInProcessNeedNoContainer(t *testing.T) {
	healthy(t)
	t.Setenv("GUSTO_SANDBOX", "here")
	printed, ok := examine(machineWith(fakeRuntime{}.commands))
	if !ok {
		t.Fatalf("a runner that uses no container should not fail on one, got:\n%s", printed)
	}
	status(t, printed, "container command", Skipped)
	if !strings.Contains(printed, "GUSTO_SANDBOX=here") {
		t.Fatalf("the skip should say why, got:\n%s", printed)
	}
}

func TestACommandThatIsNotThereNeverStarted(t *testing.T) {
	if outcome := execute("gusto-no-such-program"); outcome.Started {
		t.Fatalf("a missing program should not read as started, got %+v", outcome)
	}
	outcome := execute("sh", "-c", "echo refused; exit 125")
	if !outcome.Started || outcome.Code != 125 || lastLine(outcome.Output) != "refused" {
		t.Fatalf("an exit code and its last line should come back, got %+v", outcome)
	}
}
