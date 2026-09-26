package doctor

// The names other checks wait on.
const (
	containerCommand = "container command"
	image            = "image"
	jobsDirectory    = "jobs directory"
)

// Checks is everything a runner needs, in the order a job would find out. The
// podman line comes before the runtime's own, because when it fails it names
// the cause the runtime's error only hints at.
func Checks(machine Machine) []Check {
	return []Check{
		{Name: "podman runtime directory", Try: func() Verdict { return podmanRuntimeDirectory(machine) }},
		{Name: containerCommand, Try: func() Verdict { return runtimeAnswers(machine) }},
		{Name: image, Needs: []string{containerCommand}, Try: func() Verdict { return imagePresent(machine) }},
		{Name: "job limits", Needs: []string{image}, Try: func() Verdict { return limitsAccepted(machine) }},
		{Name: "git", Try: func() Verdict { return gitOnPath(machine) }},
		{Name: jobsDirectory, Try: func() Verdict { return jobsWritable(machine) }},
		{Name: "bind mount", Needs: []string{image, jobsDirectory}, Try: func() Verdict { return mountShows(machine) }},
		{Name: "model", Try: modelAnswers},
		{Name: "adapters", Try: adaptersParse},
		{Name: "sign-in", Try: signIn},
	}
}
