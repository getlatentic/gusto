package openshell_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/openshell/openshelltest"
	"github.com/getlatentic/gusto/pkg/harness"
)

func aBox(t *testing.T) *openshell.Sandbox {
	t.Helper()
	box, err := openshell.New(openshell.Config{Binary: "openshell", Image: "runner.tar", CPU: "1", Memory: "1Gi"},
		"close-one", harness.Adapter{}, "http://host.openshell.internal:8041/model/v1", "http://127.0.0.1:8041/model/v1", "a-job-token")
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func deletions(t *testing.T, root string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "deleted.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

// limitTheGateway gives a call that never answers milliseconds instead of minutes.
func limitTheGateway(t *testing.T) {
	t.Helper()
	openshell.SetRemoveWithin(200 * time.Millisecond)
	t.Cleanup(func() { openshell.SetRemoveWithin(2 * time.Minute) })
}

func TestCloseTwiceDeletesOnceAndTheSecondReturnsNil(t *testing.T) {
	root := openshelltest.Install(t)
	box := aBox(t)
	folder := filepath.Join(t.TempDir(), filepath.Base(openshell.Workdir))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := box.Start(context.Background(), folder); err != nil {
		t.Fatal(err)
	}
	if err := box.Close(); err != nil {
		t.Fatal(err)
	}
	if err := box.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if got := deletions(t, root); len(got) != 1 {
		t.Fatalf("deleted %v, want one delete", got)
	}
}

func TestCloseAfterAStartThatFailedRemovesWhatWasMadeAndReportsNothing(t *testing.T) {
	root := openshelltest.Install(t)
	t.Setenv("OPENSHELL_FAKE_REFUSE", "sandbox create")
	box := aBox(t)
	if err := box.Start(context.Background(), t.TempDir()); err == nil {
		t.Fatal("Start worked against a gateway that refuses to create")
	}
	t.Setenv("OPENSHELL_FAKE_REFUSE", "")
	if err := box.Close(); err != nil {
		t.Fatalf("Close after a failed Start: %v", err)
	}
	if got := deletions(t, root); len(got) != 1 {
		t.Fatalf("deleted %v, want the one clean-up delete", got)
	}
}

func TestCloseOfASandboxTheGatewayDoesNotHaveReportsNothing(t *testing.T) {
	openshelltest.Install(t)
	t.Setenv("OPENSHELL_FAKE_REFUSE", "sandbox delete")
	t.Setenv("OPENSHELL_FAKE_SAYS", "sandbox not found")
	if err := aBox(t).Close(); err != nil {
		t.Fatalf("Close of a missing sandbox: %v", err)
	}
}

func TestCloseReportsADeleteTheGatewayRefusesAndMayBeTriedAgain(t *testing.T) {
	root := openshelltest.Install(t)
	t.Setenv("OPENSHELL_FAKE_REFUSE", "sandbox delete")
	box := aBox(t)
	if err := box.Close(); err == nil {
		t.Fatal("a refused delete was hidden")
	}
	t.Setenv("OPENSHELL_FAKE_REFUSE", "")
	if err := box.Close(); err != nil {
		t.Fatal(err)
	}
	if got := deletions(t, root); len(got) != 1 {
		t.Fatalf("deleted %v, want the retry's delete", got)
	}
}

func TestCloseGivesUpOnAGatewayThatNeverAnswers(t *testing.T) {
	openshelltest.Install(t)
	limitTheGateway(t)
	t.Setenv("OPENSHELL_FAKE_HANG", "sandbox delete")
	began := time.Now()
	if err := aBox(t).Close(); err == nil {
		t.Fatal("Close reported success to a gateway that never answered")
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Fatalf("Close held its caller for %v", took)
	}
}

func TestStopGivesUpOnAGatewayThatNeverAnswers(t *testing.T) {
	openshelltest.Install(t)
	limitTheGateway(t)
	t.Setenv("OPENSHELL_FAKE_HANG", "sandbox stop")
	began := time.Now()
	if err := openshell.Stop(openshell.Config{Binary: "openshell"}, "c-one"); err == nil {
		t.Fatal("Stop reported success")
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Fatalf("Stop held its caller for %v", took)
	}
}

func TestEndingATaskInAComputerGivesUpOnAGatewayThatNeverAnswers(t *testing.T) {
	openshelltest.Install(t)
	limitTheGateway(t)
	dir, _ := aTask(t)
	box := inComputer(t, dir, "c-12345678-abcdefgh", "20260929-1200-first")
	t.Setenv("OPENSHELL_FAKE_HANG", "sandbox exec")
	began := time.Now()
	if err := box.Close(); err == nil {
		t.Fatal("Close reported success to a gateway that never answered")
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Fatalf("Close held its caller for %v", took)
	}
}
