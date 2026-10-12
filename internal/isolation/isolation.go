// Package isolation chooses the strongest wall this computer can put between an agent and the person's files, and says which in
// words. A virtual machine, then a container, then nothing: a wall of a kind the computer has is never skipped for a weaker one,
// and the person is told which they have.
package isolation

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Kind is what stands between an agent and the files of the person it works for.
type Kind string

const (
	// VM is a virtual machine for each job, where the agent sees none of the computer's files but the folder it was given.
	VM Kind = "vm"
	// Container is a container for each job, which shares the computer's kernel.
	Container Kind = "container"
	// Process is the computer's own sandbox around the agent's processes: on a Mac, Seatbelt. It writes only its own folder, reads none of
	// the person's home but its own and the engine's, and reaches nothing on the network but haas, and shares the computer's kernel.
	Process Kind = "process"
	// Nothing is no wall: the agent runs as the person, and can read and change what they can.
	Nothing Kind = "none"
)

// Backend is one kind of wall, and whether this computer can make it now.
type Backend struct {
	Kind Kind
	// Ready says this computer can make it now.
	Ready bool
	// Detail is what makes it, when ready: "Docker". When not, what is missing and how to get it.
	Detail string
	// Cause is what the computer or the runtime itself said when it could not, for the log and for haas doctor, and not for a page.
	Cause string
}

// Choice is the wall a computer has: the strongest that is ready, and why each stronger one was passed over.
type Choice struct {
	Kind   Kind
	Detail string
	// Forced says the person set HAAS_SANDBOX, and nothing was probed.
	Forced bool
	// Passed is the stronger kinds this computer could not make, with what each lacks.
	Passed []Backend
}

// Host is what the probes ask of the computer. A test is any computer it describes.
type Host struct {
	GOOS     string
	Getenv   func(string) string
	Exists   func(path string) bool
	LookPath func(program string) (string, error)
	// Run is a program's output and whether it exited 0, within a few seconds.
	Run func(program string, args ...string) (string, bool)
	// MountsSocket says whether a container of the image can be given a unix socket of this computer, which is how a job reaches
	// the proxy and nothing else. A runtime that runs in a virtual machine, as Docker does on a Mac or Windows, cannot. Nil is
	// not tested.
	MountsSocket func(runtime, image string) error
}

// This is the computer haas runs on.
func This() Host {
	return Host{
		GOOS:     runtime.GOOS,
		Getenv:   os.Getenv,
		Exists:   func(path string) bool { _, err := os.Stat(path); return err == nil },
		LookPath: exec.LookPath,
		Run: func(program string, args ...string) (string, bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, program, args...).CombinedOutput()
			return strings.TrimSpace(string(out)), err == nil
		},
		MountsSocket: SocketMount(func(program string, args ...string) (string, bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, program, args...).CombinedOutput()
			return strings.TrimSpace(string(out)), err == nil
		}),
	}
}

// Choose is the strongest wall the host can make. HAAS_SANDBOX names one when a person or an operator has decided: openshell is a
// VM, here is none, and anything else, including container, is a container.
func Choose(h Host) Choice {
	switch h.Getenv("HAAS_SANDBOX") {
	case "":
	case "openshell":
		return Choice{Kind: VM, Detail: "OpenShell", Forced: true}
	case "here":
		return Choice{Kind: Nothing, Forced: true}
	case "process":
		return Choice{Kind: Process, Detail: "Seatbelt", Forced: true}
	default:
		return Choice{Kind: Container, Detail: runtimeName(h), Forced: true}
	}
	var passed []Backend
	for _, probe := range []func(Host) Backend{virtualMachine, container, processSandbox} {
		backend := probe(h)
		if backend.Ready {
			return Choice{Kind: backend.Kind, Detail: backend.Detail, Passed: passed}
		}
		passed = append(passed, backend)
	}
	return Choice{Kind: Nothing, Passed: passed}
}

// Words says what a choice means for the person, in one sentence.
func (c Choice) Words() string {
	switch c.Kind {
	case VM:
		return "Each job runs in a virtual machine (" + c.Detail + "): it sees only the folder it was given."
	case Container:
		return "Each job runs in a container (" + c.Detail + "): it sees only the folder it was given, and shares this computer's kernel."
	case Process:
		return "Each job runs behind this computer's own sandbox (" + c.Detail + "): it can write only its own folder, cannot read the rest of your home, and reaches nothing on the network but haas. It shares this computer's kernel."
	}
	return "Nothing stands between an agent and your files: it runs as you, and can read and change what you can."
}

// Short is the choice in a few words, for a line on a page.
func (c Choice) Short() string {
	switch c.Kind {
	case VM:
		return "a virtual machine"
	case Container:
		return "a container"
	case Process:
		return "a process sandbox"
	}
	return "no sandbox"
}

// runtimeName is the container command the host uses: HAAS_CONTAINER, or docker, or podman.
func runtimeName(h Host) string {
	if name := h.Getenv("HAAS_CONTAINER"); name != "" {
		return name
	}
	for _, name := range []string{"docker", "podman"} {
		if _, err := h.LookPath(name); err == nil {
			return name
		}
	}
	return "docker"
}
