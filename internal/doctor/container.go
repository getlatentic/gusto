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
func runtimeAnswers(machine Machine) Verdict {
	if inProcess() {
		return skip("GUSTO_SANDBOX=here and process run jobs in this process, not in a container")
	}
	runtime := sandbox.Runtime()
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

// imagePresent looks only on this machine. A job whose image is missing pulls
// it in the middle of its start, and gusto:dev is in no registry, so for the
// default that pull finds nothing and the job fails with exit 125.
func imagePresent(machine Machine) Verdict {
	outcome := machine.Commands(sandbox.Runtime(), "image", "inspect", sandbox.Image())
	if outcome.Code != 0 {
		return fail(sandbox.Image()+" is not on this machine",
			"build it with `"+sandbox.Runtime()+" build -t "+sandbox.Image()+" .` or pull it, or set GUSTO_IMAGE to one that is here")
	}
	return pass(sandbox.Image() + " is here")
}

// limitsAccepted starts a container under the job's own limits. The runtime
// checks them only at a start, and a cap it refuses fails every job with exit
// 125 and an empty out/.
func limitsAccepted(machine Machine) Verdict {
	limits := sandbox.Limits()
	outcome := machine.Commands(sandbox.Runtime(), start(limits, "true")...)
	if outcome.Code != 0 {
		return fail(fmt.Sprintf("%s refused %s with exit %d: %s",
			sandbox.Runtime(), strings.Join(limits, " "), outcome.Code, lastLine(outcome.Output)),
			"set GUSTO_JOB_MEMORY and GUSTO_JOB_CPUS to what the runtime accepts and this machine has")
	}
	return pass("a container starts under " + strings.Join(limits, " "))
}

// mountShows leaves a file in the jobs directory and looks for it inside a
// container. A runtime that does not share a path mounts an empty directory in
// its place rather than refusing, so only the file's presence says the mount
// works.
func mountShows(machine Machine) Verdict {
	absolute, err := filepath.Abs(JobsDirectory())
	if err != nil {
		return fail(err.Error(), "set GUSTO_JOBS to a path this user can resolve")
	}
	marker, err := os.CreateTemp(absolute, ".doctor-*")
	if err != nil {
		return fail("could not leave a file in "+absolute+": "+err.Error(), "make the jobs directory writable by this user")
	}
	marker.Close()
	defer os.Remove(marker.Name())
	mount := []string{"-v", absolute + ":/job"}
	outcome := machine.Commands(sandbox.Runtime(), start(mount, "test -f /job/"+filepath.Base(marker.Name()))...)
	switch outcome.Code {
	case 0:
		return pass("the container sees what is in " + absolute)
	case 1:
		return fail("the container sees an empty directory at "+absolute,
			"share "+absolute+" with the container runtime, or set GUSTO_JOBS to a path it shares")
	default:
		return fail(fmt.Sprintf("a container with %s mounted exited %d: %s", absolute, outcome.Code, lastLine(outcome.Output)),
			"run the same mount by hand with "+sandbox.Runtime()+" and read what it says")
	}
}

// start is a container run that never pulls and is removed afterwards, with the
// flags under test and one shell command.
func start(flags []string, command string) []string {
	args := append([]string{"run", "--rm", "--pull=never"}, flags...)
	return append(args, "--entrypoint", "sh", sandbox.Image(), "-c", command)
}

func inProcess() bool {
	sandbox := os.Getenv("GUSTO_SANDBOX")
	return sandbox == "here" || sandbox == "process"
}
