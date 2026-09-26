package doctor

import (
	"os"
)

// JobsDirectory is where haas serve keeps jobs, and so where a job's directory
// is mounted from.
func JobsDirectory() string {
	if dir := os.Getenv("HAAS_JOBS"); dir != "" {
		return dir
	}
	return "jobs"
}

// gitOnPath matters because the runner clones outside the box, so a repository
// credential never goes in with the agent. The image having git is not enough.
func gitOnPath(machine Machine) Verdict {
	path, err := machine.LookPath("git")
	if err != nil {
		return fail("git is not on PATH", "install git where haas runs; the runner clones before the container starts")
	}
	return pass("git is at " + path)
}

// jobsWritable checks the directory every job is written into. Serve opens it
// at start and each job writes its evidence there.
func jobsWritable(machine Machine) Verdict {
	dir := JobsDirectory()
	info, err := os.Stat(dir)
	switch {
	case err != nil:
		return fail(dir+" does not exist", "create it, or set HAAS_JOBS to the directory jobs should live in")
	case !info.IsDir():
		return fail(dir+" is not a directory", "set HAAS_JOBS to a directory")
	}
	if err := machine.Writable(dir); err != nil {
		return fail(dir+" is not writable by this user: "+err.Error(),
			"give this user write access to "+dir+", and under systemd name it in ReadWritePaths")
	}
	return pass(dir + " is writable")
}
