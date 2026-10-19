package doctor

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestARuntimeThatIsNotInstalledIsNamed(t *testing.T) {
	healthy(t)
	printed, ok := examine(machineWith(fakeRuntime{}.commands))
	if ok {
		t.Fatal("no runtime means no job")
	}
	failed(t, printed, "container command", "GUSTO_CONTAINER")
	status(t, printed, "image", Skipped)
	status(t, printed, "bind mount", Skipped)
}

func TestARuntimeWhoseDaemonDoesNotAnswerIsNamed(t *testing.T) {
	healthy(t)
	runtime := answering()
	runtime.info = Outcome{Started: true, Code: 1, Output: "Cannot connect to the Docker daemon"}
	printed, _ := examine(machineWith(runtime.commands))
	failed(t, printed, "container command", "start the docker daemon")
}

func TestAnImageThatIsNotHereIsNamed(t *testing.T) {
	healthy(t)
	t.Setenv("GUSTO_IMAGE", "gusto:missing")
	runtime := answering()
	runtime.inspect = Outcome{Started: true, Code: 1}
	printed, _ := examine(machineWith(runtime.commands))
	failed(t, printed, "image", "build -t gusto:missing")
}

// The cap the runtime refuses is the one ready does not catch, because ready
// starts its container with no limits at all.
func TestAMemoryCapTheRuntimeRefusesIsNamed(t *testing.T) {
	healthy(t)
	t.Setenv("GUSTO_JOB_MEMORY", "4m")
	printed, ok := examine(machineWith(refusingTinyMemory))
	if ok {
		t.Fatal("every job would exit 125")
	}
	failed(t, printed, "task limits", "GUSTO_JOB_MEMORY")
	for _, said := range []string{"--memory=4m", "exit 125", "Minimum memory limit allowed is 6MB"} {
		if !strings.Contains(printed, said) {
			t.Fatalf("the failure should say %q, got:\n%s", said, printed)
		}
	}
}

func TestTheLimitsTriedAreTheOnesAJobRunsUnder(t *testing.T) {
	healthy(t)
	t.Setenv("GUSTO_JOB_CPUS", "2")
	t.Setenv("GUSTO_JOB_MEMORY", "900m")
	var seen []string
	record := func(program string, args ...string) Outcome {
		if args[0] == "run" && !slices.Contains(args, "-v") {
			seen = args
		}
		return answering().commands(program, args...)
	}
	examine(machineWith(record))
	for _, flag := range []string{"--cpus=2", "--memory=900m", "--pids-limit=512", "--pull=never"} {
		if !slices.Contains(seen, flag) {
			t.Fatalf("the limits container should carry %s, got %v", flag, seen)
		}
	}
}

func TestAMountThatHandsBackAnEmptyDirectoryIsNamed(t *testing.T) {
	healthy(t)
	runtime := answering()
	runtime.mount = Outcome{Started: true, Code: 1}
	printed, ok := examine(machineWith(runtime.commands))
	if ok {
		t.Fatal("a job would start in an empty directory")
	}
	failed(t, printed, "bind mount", "share")
	if !strings.Contains(printed, "empty directory") {
		t.Fatalf("the failure should say what the container sees, got:\n%s", printed)
	}
}

func TestTheMountLooksForAFileThatIsReallyThere(t *testing.T) {
	healthy(t)
	jobs := os.Getenv("GUSTO_JOBS")
	var present bool
	look := func(program string, args ...string) Outcome {
		if slices.Contains(args, "-v") {
			marker := args[len(args)-1][len("test -f /job/"):]
			_, err := os.Stat(filepath.Join(jobs, marker))
			present = err == nil
		}
		return answering().commands(program, args...)
	}
	examine(machineWith(look))
	if !present {
		t.Fatal("the file the container looks for should exist on the host while it looks")
	}
	if left, _ := os.ReadDir(jobs); len(left) != 0 {
		t.Fatalf("the check should leave the jobs directory as it found it, left %v", left)
	}
}

func TestMissingGitIsNamed(t *testing.T) {
	healthy(t)
	machine := machineWith(answering().commands)
	machine.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	printed, _ := examine(machine)
	failed(t, printed, "git", "install git")
}

func TestAJobsDirectoryThatIsNotWritableIsNamed(t *testing.T) {
	healthy(t)
	machine := machineWith(answering().commands)
	machine.Writable = func(string) error { return errDenied }
	printed, _ := examine(machine)
	failed(t, printed, "tasks directory", "ReadWritePaths")
	status(t, printed, "bind mount", Skipped)
}

func TestAJobsDirectoryThatIsNotThereIsNamed(t *testing.T) {
	healthy(t)
	t.Setenv("GUSTO_JOBS", filepath.Join(t.TempDir(), "absent"))
	printed, _ := examine(machineWith(answering().commands))
	failed(t, printed, "tasks directory", "set GUSTO_JOBS")
}

func TestAWritableJobsDirectoryIsWritable(t *testing.T) {
	if err := canWrite(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := canWrite(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a directory that is not there cannot be written")
	}
}

func TestAModelThatAnswersPasses(t *testing.T) {
	healthy(t)
	model := httptest.NewServer(http.NotFoundHandler())
	defer model.Close()
	t.Setenv("GUSTO_MODEL_URL", model.URL+"/v1")
	printed, _ := examine(machineWith(answering().commands))
	status(t, printed, "model", Pass)
}

func TestAModelThatDoesNotAnswerIsNamed(t *testing.T) {
	healthy(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	t.Setenv("GUSTO_MODEL_URL", "http://"+address+"/v1")
	printed, _ := examine(machineWith(answering().commands))
	failed(t, printed, "model", "OLLAMA_HOST=0.0.0.0")
}

func TestNoModelURLIsSkippedAndSaysWhy(t *testing.T) {
	healthy(t)
	printed, _ := examine(machineWith(answering().commands))
	status(t, printed, "model", Skipped)
	if !strings.Contains(printed, "GUSTO_MODEL_URL is not set") {
		t.Fatalf("the skip should say why, got:\n%s", printed)
	}
}

func TestAdaptersOfYourOwnMustParse(t *testing.T) {
	healthy(t)
	dir := t.TempDir()
	t.Setenv("GUSTO_HARNESSES", dir)
	write(t, filepath.Join(dir, "mine.json"), `{"name":"mine","program":"mine","argv":["{brief}"]}`)
	printed, _ := examine(machineWith(answering().commands))
	status(t, printed, "adapters", Pass)

	write(t, filepath.Join(dir, "broken.json"), `{"name":`)
	write(t, filepath.Join(dir, "empty.json"), `{"name":"empty"}`)
	printed, _ = examine(machineWith(answering().commands))
	failed(t, printed, "adapters", "names at least a program")
	for _, named := range []string{"broken:", "empty: no program"} {
		if !strings.Contains(printed, named) {
			t.Fatalf("the failure should name %q, got:\n%s", named, printed)
		}
	}
}

func TestAnAdapterDirectoryThatIsNotThereIsNamed(t *testing.T) {
	healthy(t)
	t.Setenv("GUSTO_HARNESSES", filepath.Join(t.TempDir(), "absent"))
	printed, _ := examine(machineWith(answering().commands))
	failed(t, printed, "adapters", "set GUSTO_HARNESSES")
}

// The failure that took a whole deploy to find: ProtectHome hid /run/user, and
// rootless podman could not reach its runtime directory.
func TestPodmanThatCannotReachItsRuntimeDirectoryIsNamed(t *testing.T) {
	healthy(t)
	t.Setenv("GUSTO_CONTAINER", "podman")
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "hidden"))
	printed, _ := examine(machineWith(answering().commands))
	failed(t, printed, "podman runtime directory", "ProtectHome")

	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	printed, _ = examine(machineWith(answering().commands))
	status(t, printed, "podman runtime directory", Pass)
}

func TestThePodmanCheckOnlyAppliesToRootlessPodman(t *testing.T) {
	healthy(t)
	printed, _ := examine(machineWith(answering().commands))
	status(t, printed, "podman runtime directory", Skipped)

	t.Setenv("GUSTO_CONTAINER", "podman")
	root := machineWith(answering().commands)
	root.User = func() int { return 0 }
	printed, _ = examine(root)
	status(t, printed, "podman runtime directory", Skipped)
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHardwareVirtualizationIsSkippedForADeploymentThatRunsJobsInContainers(t *testing.T) {
	t.Setenv("GUSTO_SANDBOX", "")
	if verdict := sandboxHardware(); verdict.Status != Skipped {
		t.Errorf("a deployment with no sandboxes was asked for hardware virtualization: %+v", verdict)
	}
}

func TestHardwareVirtualizationIsDemandedOfAMachineThatRunsSandboxesAndHasNone(t *testing.T) {
	t.Setenv("GUSTO_SANDBOX", "openshell")
	t.Setenv("GUSTO_OPENSHELL_REMOTE", "1")
	if verdict := sandboxHardware(); verdict.Status != Pass {
		t.Errorf("a machine whose gateway is elsewhere was refused: %+v", verdict)
	}
}

// The doctor says which wall gusto would put between an agent and the files, in words, and a computer with none is told so without
// being called unhealthy.
func TestTheDoctorSaysWhichWallAnAgentRunsBehind(t *testing.T) {
	t.Setenv("GUSTO_SANDBOX", "")
	only := func(programs ...string) func(string) (string, error) {
		return func(program string) (string, error) {
			if slices.Contains(programs, program) {
				return "/bin/" + program, nil
			}
			return "", errors.New("not found")
		}
	}
	withDocker := Machine{LookPath: only("docker"), Commands: func(string, ...string) Outcome { return Outcome{Started: true} }}
	if got := sandboxChoice(withDocker); got.Status != Pass || !strings.Contains(got.Said, "in a container (docker)") {
		t.Errorf("%+v", got)
	}
	bare := Machine{LookPath: only(), Commands: func(string, ...string) Outcome { return Outcome{} }}
	got := sandboxChoice(bare)
	if got.Status != Pass || !strings.Contains(got.Said, "Nothing stands between an agent and your files") || !strings.Contains(got.Remedy, "container: neither Docker nor Podman is installed") {
		t.Errorf("%+v", got)
	}
}
