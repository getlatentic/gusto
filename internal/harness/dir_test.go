package harness

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// A turn that names a folder of the workspace starts there, and one that names a way out of it starts at the root.
func TestAHarnessStartsInTheFolderTheTurnNames(t *testing.T) {
	adapter := Adapter{Name: "x", Program: "sh"}
	root := filepath.FromSlash("/job/workspace")
	for dir, want := range map[string]string{"": root, "src/app": filepath.Join(root, "src", "app"), "../outside": root, "/etc": root} {
		command := adapter.CommandIn(context.Background(), job.Spec{Dir: dir}, root, Env{})
		if command.Dir != want {
			t.Errorf("the folder %q started in %q, not %q", dir, command.Dir, want)
		}
	}
}
