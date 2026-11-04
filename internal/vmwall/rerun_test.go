package vmwall

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A new release's root file system replaces the one an install before kept, which has the same name.
func TestANewReleasesImageReplacesTheOneKeptBefore(t *testing.T) {
	c, env := newComputer(t, "arm64")
	plan, err := Prepare(context.Background(), env, c.root, c.user, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	image, sum := archiveOf(t, "rootfs.tar", "the next release's image")
	c.assets[RootfsAsset("arm64")] = image
	c.assets[RootfsAsset("arm64")+".sha256"] = []byte(sum + "  " + strconv.Itoa(len(image)) + "  " + RootfsAsset("arm64") + "\n")
	again, err := Prepare(context.Background(), env, c.root, c.user, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if kept, _ := os.ReadFile(again.imagePath()); string(kept) != string(image) {
		t.Error("the image of the release before is still the one tasks start from")
	}
}

// A re-run that does not finish leaves no record that gusto would use, and stops the gateway, which nothing uses then.
func TestAReRunThatFailsIsNotUsedAndStopsTheGateway(t *testing.T) {
	c, env := newComputer(t, "arm64")
	stopped := false
	env.Stop = func(Layout) error { stopped = true; return nil }
	plan, err := Prepare(context.Background(), env, c.root, c.user, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := Installed(plan.Layout); !ok {
		t.Fatal("the first install is not used")
	}
	c.failRun = "sandbox list"
	was := startupWait
	startupWait = 50 * time.Millisecond
	t.Cleanup(func() { startupWait = was })
	if err := plan.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "Waiting for the gateway") {
		t.Fatalf("the re-run did not say where it stopped: %v", err)
	}
	if _, ok := Installed(plan.Layout); ok {
		t.Error("a re-run that did not finish left the record of the install before, beside its new programs")
	}
	if !stopped {
		t.Error("the gateway is left running with nothing that uses it")
	}
}
