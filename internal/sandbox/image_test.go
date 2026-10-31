package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// imageRuntime is a container runtime that has the image once something was loaded, and keeps what it was told.
func imageRuntime(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in runtime is a shell script")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
dir="` + dir + `"
echo "$@" >> "$dir/said"
case "$1" in
image) [ -f "$dir/tagged" ] ;;
load) cat > "$dir/archive"; echo "Loaded image: gusto-runner:latest" ;;
tag) touch "$dir/tagged" ;;
esac
`
	path := filepath.Join(dir, "runtime")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// A machine without the runner image fetches it from its source, loads it, and names it as the image tasks run in; one
// that has it fetches nothing.
func TestAMissingRunnerImageIsFetchedLoadedAndNamedOnce(t *testing.T) {
	runtimePath := imageRuntime(t)
	var fetched atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched.Add(1)
		w.Write([]byte("the image's layers"))
	}))
	defer source.Close()
	t.Setenv("GUSTO_CONTAINER", runtimePath)
	t.Setenv("GUSTO_IMAGE", "gusto-runner:abc123")
	t.Setenv(ImageSource, source.URL+"/image/abc123.docker.tar.gz")

	if err := ensureImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(runtimePath)
	if archive, _ := os.ReadFile(filepath.Join(dir, "archive")); string(archive) != "the image's layers" {
		t.Errorf("the runtime was loaded with %q", archive)
	}
	if said, _ := os.ReadFile(filepath.Join(dir, "said")); !strings.Contains(string(said), "tag gusto-runner:latest gusto-runner:abc123") {
		t.Errorf("the loaded image was not named for the tasks:\n%s", said)
	}
	if err := ensureImage(context.Background()); err != nil || fetched.Load() != 1 {
		t.Errorf("an image that is here was fetched again: %d fetches, %v", fetched.Load(), err)
	}
}

// With no source a missing image is left for the container's start to report, and a source that fails says why.
func TestARunnerImageWithNoSourceIsLeftAndAFailedFetchSaysWhy(t *testing.T) {
	t.Setenv("GUSTO_CONTAINER", imageRuntime(t))
	t.Setenv("GUSTO_IMAGE", "gusto-runner:abc123")
	t.Setenv(ImageSource, "")
	if err := ensureImage(context.Background()); err != nil {
		t.Errorf("with no source: %v", err)
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	defer gone.Close()
	t.Setenv(ImageSource, gone.URL+"/image/abc123.docker.tar.gz")
	if err := ensureImage(context.Background()); err == nil || !strings.Contains(err.Error(), "could not be fetched") {
		t.Errorf("a source that has no image: %v", err)
	}
	t.Setenv(ImageSource, "ftp://elsewhere/x")
	if err := ensureImage(context.Background()); err == nil {
		t.Error("a source of another kind was taken")
	}
}
