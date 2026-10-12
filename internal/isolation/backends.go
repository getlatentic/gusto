package isolation

import "strings"

// virtualMachine is an OpenShell sandbox: a VM for each job, which needs the hardware, the openshell command, and a gateway that
// answers with an image that carries the engines.
func virtualMachine(h Host) Backend {
	no := func(why string) Backend { return Backend{Kind: VM, Detail: why} }
	switch h.GOOS {
	case "linux":
		if !h.Exists("/dev/kvm") {
			return no("this computer has no /dev/kvm, which virtual machines need")
		}
	case "darwin":
		if out, ok := h.Run("sysctl", "-n", "kern.hv_support"); !ok || out != "1" {
			return no("this Mac does not offer hardware virtualization")
		}
	default:
		return no("virtual machines for jobs are not supported on " + h.GOOS)
	}
	binary := h.Getenv("HAAS_OPENSHELL")
	if binary == "" {
		binary = "openshell"
	}
	if _, err := h.LookPath(binary); err != nil {
		return no("OpenShell is not installed")
	}
	if h.Getenv("HAAS_OPENSHELL_IMAGE") == "" {
		return no("HAAS_OPENSHELL_IMAGE names no image that carries the engines")
	}
	if _, ok := h.Run(binary, "sandbox", "list"); !ok {
		return no("no OpenShell gateway answers")
	}
	return Backend{Kind: VM, Ready: true, Detail: "OpenShell"}
}

// oneLine is the first line of what a program said, which is where a runtime puts why it refused: a usage hint follows it.
func oneLine(said string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(said), "\n")
	return strings.TrimSpace(line)
}

// container is a container for each job, which needs a runtime that answers and the runner image on this computer. An image is
// not pulled for the person: it is more than a gigabyte, and that is theirs to start.
func container(h Host) Backend {
	no := func(why string) Backend { return Backend{Kind: Container, Detail: why} }
	if h.GOOS == "windows" {
		return no("containers for jobs are not supported on Windows yet")
	}
	name := runtimeName(h)
	if _, err := h.LookPath(name); err != nil {
		return no("neither Docker nor Podman is installed")
	}
	if _, ok := h.Run(name, "info"); !ok {
		return no(name + " is installed and does not answer: start it")
	}
	image := h.Getenv("HAAS_IMAGE")
	if image == "" {
		image = "haas:dev"
	}
	if _, ok := h.Run(name, "image", "inspect", image); !ok {
		return no(name + " answers, and the runner image " + image + " is not on this computer")
	}
	if h.MountsSocket != nil {
		if err := h.MountsSocket(name, image); err != nil {
			refused := no(name + " cannot be given the socket a job reaches the proxy through: Docker on a Mac or Windows runs in a virtual machine that cannot share one")
			refused.Cause = oneLine(err.Error())
			return refused
		}
	}
	return Backend{Kind: Container, Ready: true, Detail: name}
}
