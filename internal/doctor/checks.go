package doctor

// The names other checks wait on.
const (
	containerCommand = "container command"
	image            = "image"
	jobsDirectory    = "tasks directory"
)

// Checks is everything a runner needs, in the order a task would find out. The
// podman line comes before the runtime's own, because when it fails it names
// the cause the runtime's error only hints at.
func Checks(machine Machine, s Settings) []Check {
	box := s.Containers
	return []Check{
		{Name: "sandbox", Try: func() Verdict { return sandboxChoice(machine, s.Sandbox) }},
		{Name: "podman runtime directory", Try: func() Verdict { return podmanRuntimeDirectory(machine, box.Runtime) }},
		{Name: containerCommand, Try: func() Verdict { return runtimeAnswers(machine, s) }},
		{Name: image, Needs: []string{containerCommand}, Try: func() Verdict { return imagePresent(machine, box) }},
		{Name: "task limits", Needs: []string{image}, Try: func() Verdict { return limitsAccepted(machine, box) }},
		{Name: "git", Try: func() Verdict { return gitOnPath(machine) }},
		{Name: jobsDirectory, Try: func() Verdict { return jobsWritable(machine, s.Jobs) }},
		{Name: "bind mount", Needs: []string{image, jobsDirectory}, Try: func() Verdict { return mountShows(machine, box, s.Jobs) }},
		{Name: "hardware virtualization", Try: func() Verdict { return sandboxHardware(s) }},
		{Name: "model", Try: func() Verdict { return modelAnswers(s.ModelURL) }},
		{Name: "adapters", Try: func() Verdict { return adaptersParse(s.Harnesses) }},
		{Name: "sign-in", Try: func() Verdict { return signIn(s.Password) }},
	}
}
