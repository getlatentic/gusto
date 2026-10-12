package isolation

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// computer is a host that has the programs and files named, and answers the commands in answers by their first words.
func computer(goos string, env map[string]string, has []string, answers map[string]string) Host {
	return Host{
		GOOS:   goos,
		Getenv: func(name string) string { return env[name] },
		Exists: func(path string) bool { return slices.Contains(has, path) },
		LookPath: func(program string) (string, error) {
			if slices.Contains(has, program) {
				return "/bin/" + program, nil
			}
			return "", errors.New("not found")
		},
		Run: func(program string, args ...string) (string, bool) {
			out, ok := answers[strings.Join(append([]string{program}, args...), " ")]
			return out, ok
		},
	}
}

func TestTheStrongestWallTheComputerCanMakeIsChosen(t *testing.T) {
	vm := map[string]string{"HAAS_OPENSHELL_IMAGE": "rootfs.tar"}
	tests := map[string]struct {
		host Host
		want Kind
	}{
		"a Linux machine with KVM, OpenShell and a gateway": {computer("linux", vm, []string{"/dev/kvm", "openshell", "docker"},
			map[string]string{"openshell sandbox list": "", "docker info": "x", "docker image inspect haas:dev": "sha256:1"}), VM},
		"a Mac with hardware virtualization, OpenShell and a gateway": {computer("darwin", vm, []string{"openshell"},
			map[string]string{"sysctl -n kern.hv_support": "1", "openshell sandbox list": ""}), VM},
		"Docker and the image, and no OpenShell": {computer("darwin", nil, []string{"docker"},
			map[string]string{"docker info": "x", "docker image inspect haas:dev": "sha256:1"}), Container},
		"Podman and the image": {computer("linux", nil, []string{"podman"},
			map[string]string{"podman info": "x", "podman image inspect haas:dev": "sha256:1"}), Container},
		"Docker with no image":          {computer("linux", nil, []string{"docker"}, map[string]string{"docker info": "x"}), Nothing},
		"a Docker that does not answer": {computer("linux", nil, []string{"docker"}, nil), Nothing},
		"OpenShell with no gateway, and Docker": {computer("linux", vm, []string{"/dev/kvm", "openshell", "docker"},
			map[string]string{"docker info": "x", "docker image inspect haas:dev": "sha256:1"}), Container},
		"a bare computer": {computer("windows", nil, nil, nil), Nothing},
		"Windows with Docker and the image": {computer("windows", nil, []string{"docker"},
			map[string]string{"docker info": "x", "docker image inspect haas:dev": "sha256:1"}), Nothing},
	}
	for name, test := range tests {
		if got := Choose(test.host); got.Kind != test.want {
			t.Errorf("%s: chose %s (%s), want %s", name, got.Kind, got.Detail, test.want)
		}
	}
}

// Docker in a virtual machine, as on a Mac, answers and has the image and still cannot be given the socket a job reaches the proxy
// through, so it is not a wall haas can use, and the person is told why.
func TestADockerThatCannotMountASocketIsNotAWall(t *testing.T) {
	host := computer("darwin", nil, []string{"docker"}, map[string]string{"docker info": "x", "docker image inspect haas:dev": "sha256:1"})
	host.MountsSocket = func(string, string) error {
		return errors.New("docker: Error response from daemon: error while creating mount source path: operation not supported\nRun 'docker run --help' for more information")
	}
	got := Choose(host)
	if got.Kind != Nothing || len(got.Passed) != 2 || !strings.Contains(got.Passed[1].Cause, "operation not supported") || strings.Contains(got.Passed[1].Cause, "--help") || !strings.Contains(got.Passed[1].Detail, "cannot share one") || strings.Contains(got.Passed[1].Detail, "operation not supported") {
		t.Errorf("%+v", got)
	}
	host.MountsSocket = func(string, string) error { return nil }
	if got := Choose(host); got.Kind != Container {
		t.Errorf("a Docker that mounts the socket was not chosen: %+v", got)
	}
}

func TestWhatIsMissingIsSaidForEachStrongerWall(t *testing.T) {
	got := Choose(computer("linux", nil, []string{"docker"}, map[string]string{"docker info": "x"}))
	if len(got.Passed) != 2 || got.Passed[0].Kind != VM || got.Passed[1].Kind != Container {
		t.Fatalf("%+v", got.Passed)
	}
	if !strings.Contains(got.Passed[0].Detail, "/dev/kvm") || !strings.Contains(got.Passed[1].Detail, "runner image haas:dev is not on this computer") {
		t.Errorf("what is missing is not said: %+v", got.Passed)
	}
}

func TestAPersonWhoSetsTheSandboxIsNotSecondGuessed(t *testing.T) {
	for setting, want := range map[string]Kind{"here": Nothing, "openshell": VM, "container": Container, "anything": Container} {
		got := Choose(computer("linux", map[string]string{"HAAS_SANDBOX": setting}, nil, nil))
		if got.Kind != want || !got.Forced || len(got.Passed) != 0 {
			t.Errorf("HAAS_SANDBOX=%s: %+v", setting, got)
		}
	}
}

func TestEachWallIsDescribedInWordsThatSayWhatItMeans(t *testing.T) {
	for _, test := range []struct {
		choice Choice
		short  string
		words  string
	}{
		{Choice{Kind: VM, Detail: "OpenShell"}, "a virtual machine", "virtual machine (OpenShell)"},
		{Choice{Kind: Container, Detail: "Docker"}, "a container", "shares this computer's kernel"},
		{Choice{Kind: Nothing}, "no sandbox", "can read and change what you can"},
	} {
		if test.choice.Short() != test.short || !strings.Contains(test.choice.Words(), test.words) {
			t.Errorf("%s: %q, %q", test.choice.Kind, test.choice.Short(), test.choice.Words())
		}
	}
}
