package box_test

import (
	"testing"

	"github.com/getlatentic/gusto/internal/openshell"
	internal "github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/box"
)

// The sandbox on a gateway is a Box, and the runner's own name for the interface is this one.
var (
	_ box.Box      = (*openshell.Sandbox)(nil)
	_ box.Box      = internal.Box(nil)
	_ internal.Box = box.Box(nil)
)

func TestTheRunnerKeepsTheNamesOfTheFilesTheGuardLeaves(t *testing.T) {
	if internal.GuardEvents != box.GuardEvents || internal.GuardPolicy != box.GuardPolicy {
		t.Fatal("the runner and the box name different files")
	}
	if box.GuardEvents != "sandbox-events.log" || box.GuardPolicy != "sandbox-policy.json" {
		t.Fatalf("%q %q: a record written before this change would not be read", box.GuardEvents, box.GuardPolicy)
	}
}
