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
		return no("virtual machines for tasks are not supported on " + h.GOOS)
	}
	binary := h.Getenv("GUSTO_OPENSHELL")
	if binary == "" {
		binary = "openshell"
	}
	if _, err := h.LookPath(binary); err != nil {
		return no("OpenShell is not installed")
	}
	if h.Getenv("GUSTO_OPENSHELL_IMAGE") == "" {
		return no("GUSTO_OPENSHELL_IMAGE names no image that carries the engines")
	}
	list := []string{"sandbox", "list"}
	if gateway := h.Getenv("GUSTO_OPENSHELL_GATEWAY"); gateway != "" {
		list = append([]string{"--gateway", gateway}, list...)
	}
	if _, ok := h.Run(binary, list...); !ok {
		return no("no OpenShell gateway answers")
	}
	return Backend{Kind: VM, Ready: true, Detail: "OpenShell"}
}

// processName is what a computer calls its own sandbox around a process.
func processName(h Host) string {
	if h.GOOS == "linux" {
		return "Landlock"
	}
	return "Seatbelt"
}

// processSandbox is the computer's own sandbox around a process: Seatbelt on a Mac, which is nothing to install, and Landlock on
// Linux, which gusto applies itself. It is tried with a real process, and not assumed from the command or the kernel being there.
func processSandbox(h Host) Backend {
	no := func(why string) Backend { return Backend{Kind: Process, Detail: why} }
	switch h.GOOS {
	case "darwin":
		if out, ok := h.Run("/usr/bin/sandbox-exec", "-p", "(version 1)(allow default)", "/usr/bin/true"); !ok {
			refused := no("sandbox-exec does not run on this Mac")
			refused.Cause = oneLine(out)
			return refused
		}
		return Backend{Kind: Process, Ready: true, Detail: "Seatbelt"}
	case "linux":
		return landlock(h, no)
	}
	return no("a process sandbox is for macOS and Linux only so far")
}

// landlock is Linux's sandbox, tried by gusto starting itself behind it. Limiting the network to gusto's proxy needs a kernel of 6.7 or
// later, and a kernel without that is not given the wall unless the person has left the network open.
func landlock(h Host, no func(string) Backend) Backend {
	if h.Self == "" {
		return no("gusto does not know where its own program is")
	}
	args := []string{"sandbox-run", "--check"}
	if h.Getenv("GUSTO_JOB_NETWORK") != "open" {
		args = append(args, "--network")
	}
	out, ok := h.Run(h.Self, args...)
	if !ok {
		refused := no("this kernel's Landlock cannot hold a task to its folder and gusto's proxy: it needs Linux 6.7 or later")
		refused.Cause = oneLine(out)
		return refused
	}
	return Backend{Kind: Process, Ready: true, Detail: "Landlock"}
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
		return no("containers for tasks are not supported on Windows yet")
	}
	name := runtimeName(h)
	if _, err := h.LookPath(name); err != nil {
		return no("neither Docker nor Podman is installed")
	}
	if _, ok := h.Run(name, "info"); !ok {
		return no(name + " is installed and does not answer: start it")
	}
	image := h.Getenv("GUSTO_IMAGE")
	if image == "" {
		image = "gusto:dev"
	}
	if _, ok := h.Run(name, "image", "inspect", image); !ok {
		return no(name + " answers, and the runner image " + image + " is not on this computer")
	}
	if h.MountsSocket != nil {
		if err := h.MountsSocket(name, image); err != nil {
			refused := no(name + " cannot be given the socket a task reaches the proxy through: Docker on a Mac or Windows runs in a virtual machine that cannot share one")
			refused.Cause = oneLine(err.Error())
			return refused
		}
	}
	return Backend{Kind: Container, Ready: true, Detail: name}
}
