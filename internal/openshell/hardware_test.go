package openshell

import (
	"strings"
	"testing"
)

func TestAMachineWithNoKVMIsRefusedForSandboxesAndTheRefusalSaysWhy(t *testing.T) {
	none := func(string) bool { return false }
	err := noHardware("linux", none, false)
	if err == nil || !strings.Contains(err.Error(), "/dev/kvm") || !strings.Contains(err.Error(), "nested virtualization") {
		t.Fatalf("a Linux machine with no KVM was not refused with the reason: %v", err)
	}
}

func TestAMachineThatCanMakeVMsOrNeedsNoneIsNotRefused(t *testing.T) {
	has := func(path string) bool { return path == "/dev/kvm" }
	none := func(string) bool { return false }
	for name, err := range map[string]error{
		"a Linux machine with KVM":               noHardware("linux", has, false),
		"a Mac, which has Hypervisor.framework":  noHardware("darwin", none, false),
		"a machine whose gateway is another one": noHardware("linux", none, true),
	} {
		if err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
}
