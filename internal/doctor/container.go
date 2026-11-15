package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/sandbox"
)

// runtimeAnswers asks for the runtime's own report, which needs a daemon that
// answers, not only a binary on PATH.
func runtimeAnswers(machine Machine, s Settings) Verdict {
	if s.inProcess() {
		return skip("GUSTO_SANDBOX=here and process run tasks in this process, not in a container")
	}
	runtime := s.Containers.Runtime
	outcome := machine.Commands(runtime, "info")
	switch {
	case !outcome.Started:
		return fail(runtime+" is not on PATH",
			"install "+runtime+", or set GUSTO_CONTAINER to the container command this machine has")
	case outcome.Code != 0:
		return fail(runtime+" does not answer: "+lastLine(outcome.Output),
			"start the "+runtime+" daemon, or for rootless podman read the runtime directory line above")
	default:
		return pass(runtime + " answers")
	}
}

// imagePresent looks only on this machine. A task whose image is missing pulls
// it in the middle of its start, and gusto:dev is in no registry, so for the
// default that pull finds nothing and the task fails with exit 125.
func imagePresent(machine Machine, box sandbox.Containers) Verdict {
	outcome := machine.Commands(box.Runtime, "image", "inspect", box.Image)
	if outcome.Code != 0 {
		return fail(box.Image+" is not on this machine",
			"build it with `"+box.Runtime+" build -t "+box.Image+" .` or pull it, or set GUSTO_IMAGE to one that is here")
	}
	return pass(box.Image + " is here")
}

// limitsAccepted starts a container under the task's own limits. The runtime
// checks them only at a start, and a cap it refuses fails every task with exit
// 125 and an empty out/.
func limitsAccepted(machine Machine, box sandbox.Containers) Verdict {
	limits := box.Limits()
	outcome := machine.Commands(box.Runtime, start(box, limits, "true")...)
	if outcome.Code != 0 {
		return fail(fmt.Sprintf("%s refused %s with exit %d: %s",
			box.Runtime, strings.Join(limits, " "), outcome.Code, lastLine(outcome.Output)),
			"set GUSTO_TASK_MEMORY and GUSTO_TASK_CPUS to what the runtime accepts and this machine has")
	}
	return pass("a container starts under " + strings.Join(limits, " "))
}

// mountShows leaves a file in the tasks directory and looks for it inside a
// container. A runtime that does not share a path mounts an empty directory in
// its place rather than refusing, so only the file's presence says the mount
// works.
func mountShows(machine Machine, box sandbox.Containers, tasks string) Verdict {
	absolute, err := filepath.Abs(tasks)
	if err != nil {
		return fail(err.Error(), "set GUSTO_TASKS to a path this user can resolve")
	}
	marker, err := os.CreateTemp(absolute, ".doctor-*")
	if err != nil {
		return fail("could not leave a file in "+absolute+": "+err.Error(), "make the tasks directory writable by this user")
	}
	marker.Close()
	defer os.Remove(marker.Name())
	mount := []string{"-v", absolute + ":/job"}
	outcome := machine.Commands(box.Runtime, start(box, mount, "test -f /job/"+filepath.Base(marker.Name()))...)
	switch outcome.Code {
	case 0:
		return pass("the container sees what is in " + absolute)
	case 1:
		return fail("the container sees an empty directory at "+absolute,
			"share "+absolute+" with the container runtime, or set GUSTO_TASKS to a path it shares")
	default:
		return fail(fmt.Sprintf("a container with %s mounted exited %d: %s", absolute, outcome.Code, lastLine(outcome.Output)),
			"run the same mount by hand with "+box.Runtime+" and read what it says")
	}
}

// start is a container run that never pulls and is removed afterwards, with the
// flags under test and one shell command.
func start(box sandbox.Containers, flags []string, command string) []string {
	args := append([]string{"run", "--rm", "--pull=never"}, flags...)
	return append(args, "--entrypoint", "sh", box.Image, "-c", command)
}
