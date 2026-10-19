package openshell

import (
	"fmt"
	"os"
	"runtime"
)

// kvm is the device a Linux host makes hardware virtualization available through.
const kvm = "/dev/kvm"

// HostCannotMakeVMs says why this machine cannot be where an OpenShell gateway makes its microVMs, or is nil when it can.
// The gateway's VM driver needs KVM on Linux, which a cloud machine has only when it is launched with nested
// virtualization, and a gateway on another machine is told with GUSTO_OPENSHELL_REMOTE=1, since this one then needs none.
func HostCannotMakeVMs() error {
	return noHardware(runtime.GOOS, func(path string) bool { _, err := os.Stat(path); return err == nil }, os.Getenv("GUSTO_OPENSHELL_REMOTE") == "1")
}

func noHardware(goos string, exists func(string) bool, remote bool) error {
	if remote || goos != "linux" || exists(kvm) {
		return nil
	}
	return fmt.Errorf("GUSTO_SANDBOX=openshell needs hardware virtualization, and this machine has no %s, so an OpenShell gateway here "+
		"cannot make a sandbox. What makes it appear depends on where this machine runs: on your own server, turn on VT-x or AMD-V in "+
		"the BIOS and run `modprobe kvm_intel` (or kvm_amd); in a virtual machine, ask whoever runs the hypervisor to expose "+
		"hardware virtualization to it; on a cloud machine, use a type that supports nested virtualization and enable it at launch; "+
		"in a container, pass the host's device in with --device %s. If the gateway runs on another machine, set GUSTO_OPENSHELL_REMOTE=1. "+
		"docs/OPENSHELL.md has the detail", kvm, kvm)
}
