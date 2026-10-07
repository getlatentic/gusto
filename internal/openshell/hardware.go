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
// virtualization, and a gateway on another machine is told with HAAS_OPENSHELL_REMOTE=1, since this one then needs none.
func HostCannotMakeVMs() error {
	return noHardware(runtime.GOOS, func(path string) bool { _, err := os.Stat(path); return err == nil }, os.Getenv("HAAS_OPENSHELL_REMOTE") == "1")
}

func noHardware(goos string, exists func(string) bool, remote bool) error {
	if remote || goos != "linux" || exists(kvm) {
		return nil
	}
	return fmt.Errorf("HAAS_SANDBOX=openshell needs hardware virtualization, and this machine has no %s, so an OpenShell gateway here "+
		"cannot make a sandbox. Run on a host with KVM (on a cloud machine, one launched with nested virtualization), "+
		"or set HAAS_OPENSHELL_REMOTE=1 if the gateway runs on another machine", kvm)
}
