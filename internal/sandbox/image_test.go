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

	if err := EnsureImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(runtimePath)
	if archive, _ := os.ReadFile(filepath.Join(dir, "archive")); string(archive) != "the image's layers" {
		t.Errorf("the runtime was loaded with %q", archive)
	}
	if said, _ := os.ReadFile(filepath.Join(dir, "said")); !strings.Contains(string(said), "tag gusto-runner:latest gusto-runner:abc123") {
		t.Errorf("the loaded image was not named for the tasks:\n%s", said)
	}
	if err := EnsureImage(context.Background()); err != nil || fetched.Load() != 1 {
		t.Errorf("an image that is here was fetched again: %d fetches, %v", fetched.Load(), err)
	}
}

// With no source a missing image is left for the container's start to report, and a source that fails says why.
func TestARunnerImageWithNoSourceIsLeftAndAFailedFetchSaysWhy(t *testing.T) {
	t.Setenv("GUSTO_CONTAINER", imageRuntime(t))
	t.Setenv("GUSTO_IMAGE", "gusto-runner:abc123")
	t.Setenv(ImageSource, "")
	if err := EnsureImage(context.Background()); err != nil {
		t.Errorf("with no source: %v", err)
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	defer gone.Close()
	t.Setenv(ImageSource, gone.URL+"/image/abc123.docker.tar.gz")
	if err := EnsureImage(context.Background()); err == nil || !strings.Contains(err.Error(), "could not be fetched") {
		t.Errorf("a source that has no image: %v", err)
	}
	t.Setenv(ImageSource, "ftp://elsewhere/x")
	if err := EnsureImage(context.Background()); err == nil {
		t.Error("a source of another kind was taken")
	}
}

// A load whose naming failed is named on the next try, and the image is not fetched again.
func TestALoadedImageThatCouldNotBeNamedIsNamedAndNotFetchedAgain(t *testing.T) {
	runtimePath := imageRuntime(t)
	dir := filepath.Dir(runtimePath)
	script, _ := os.ReadFile(runtimePath)
	failOnce := strings.Replace(string(script), `tag) touch "$dir/tagged" ;;`,
		`tag) if [ -f "$dir/refused" ]; then touch "$dir/tagged"; else touch "$dir/refused"; echo "no room" >&2; exit 1; fi ;;
inspect) [ -f "$dir/archive" ] ;;`, 1)
	failOnce = strings.Replace(failOnce, `image) [ -f "$dir/tagged" ] ;;`, `image) if [ "$3" = "gusto-runner:latest" ]; then [ -f "$dir/archive" ]; else [ -f "$dir/tagged" ]; fi ;;`, 1)
	if err := os.WriteFile(runtimePath, []byte(failOnce), 0o755); err != nil {
		t.Fatal(err)
	}
	var fetched atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched.Add(1)
		w.Write([]byte("the image's layers"))
	}))
	defer source.Close()
	t.Setenv("GUSTO_CONTAINER", runtimePath)
	t.Setenv("GUSTO_IMAGE", "gusto-runner:abc123")
	t.Setenv(ImageSource, source.URL+"/image/abc123.docker.tar.gz")
	t.Setenv("TMPDIR", t.TempDir())
	if err := EnsureImage(context.Background()); err == nil {
		t.Fatal("a load that could not be named was taken")
	}
	if err := EnsureImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fetched.Load() != 1 {
		t.Errorf("the image was fetched %d times", fetched.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "tagged")); err != nil {
		t.Error("the loaded image was not named on the second try")
	}
}
