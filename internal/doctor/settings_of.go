package doctor

import (
	"github.com/getlatentic/gusto/internal/isolation"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// Settings are the settings of the server the checks are for, as gusto doctor read them.
type Settings struct {
	// Sandbox is what was set about the box tasks are held in, which the strongest one is chosen by.
	Sandbox isolation.Settings
	// Containers is how a task's container is started.
	Containers sandbox.Containers
	// Tasks is the directory tasks are kept in.
	Tasks string
	// ModelURL is the model a task asks directly, or nothing when none was named.
	ModelURL string
	// Harnesses is the directory of adapters of your own, or nothing.
	Harnesses string
	// Password says gusto serve has a password. What it is never reaches the checks.
	Password bool
	// OpenShellRemote says the OpenShell gateway runs on another machine.
	OpenShellRemote bool
}

// inProcess says tasks run in the server's own process, with no container.
func (s Settings) inProcess() bool {
	return s.Sandbox.Sandbox == "here" || s.Sandbox.Sandbox == "process"
}
