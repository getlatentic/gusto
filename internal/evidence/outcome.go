package evidence

// Earned is the exit code a run earned, by the one rule the runner applies when a run ends and the task index applies when it
// finds a run that ended while nobody watched. A check decides when there is one. A turn in a conversation has none, and then it
// failed when the harness failed, whatever it managed to say first, or when the proxy saw its model never answer, whatever the
// harness's exit says.
func Earned(verdict *Verdict, harnessExit int, modelSilent bool) int {
	switch {
	case verdict != nil && verdict.Passed:
		return 0
	case verdict != nil:
		return 1
	case harnessExit != 0, modelSilent:
		return 1
	}
	return 0
}
