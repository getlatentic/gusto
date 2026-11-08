package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/filelock"
)

// imageFetchTime is the most fetching and loading the image may take: a gigabyte or so from the same region.
const imageFetchTime = 15 * time.Minute

var (
	// imageMu makes one fetch at a time in this process; a task that waits for it then finds the image there.
	imageMu sync.Mutex
	// loadedAs is the name the last load gave the image, which a failed tag left as it was; imageMu guards it.
	loadedAs string
)

// EnsureImage makes the runner image present before a container is started from it, fetching and loading it from ImageSource
// when it is missing: an s3:// address read with the AWS command line, or an http(s):// one. A machine kept running across
// releases is handed a new image this way, since nothing else gives it one until it starts again. With no source, a missing
// image is left for the start to say so. The servers of one machine share its runtime, so a fetch holds a lock beside the
// runtime's other users, and the one that waited finds the image there. A load whose tag failed is only tagged again.
func (c Containers) EnsureImage(ctx context.Context) error {
	if c.ImageSource == "" {
		return nil
	}
	imageMu.Lock()
	defer imageMu.Unlock()
	if c.present(ctx, c.Image) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, imageFetchTime)
	defer cancel()
	if held, err := filelock.Wait(ctx, c.imageLock()); err == nil {
		defer held.Release()
	} else if ctx.Err() != nil {
		return err
	} else {
		slog.Warn("the runner image is fetched without the lock other servers on this machine hold", "error", err)
	}
	switch {
	case c.present(ctx, c.Image):
		return nil
	case loadedAs != "" && c.present(ctx, loadedAs):
		return c.tag(ctx, loadedAs)
	}
	return c.fetch(ctx)
}

// fetch loads the runner image from its source and names it for the tasks. imageMu is held.
func (c Containers) fetch(ctx context.Context) error {
	archive, wait, err := openImage(ctx, c.ImageSource)
	if err != nil {
		return fmt.Errorf("the runner image %s is not here and could not be fetched from %s: %w", c.Image, c.ImageSource, err)
	}
	defer archive.Close()
	loaded, err := c.loadImage(ctx, archive)
	if waitErr := wait(); err == nil {
		err = waitErr
	}
	if err != nil {
		return fmt.Errorf("the runner image %s could not be loaded from %s: %w", c.Image, c.ImageSource, err)
	}
	if loaded == c.Image {
		return nil
	}
	loadedAs = loaded
	return c.tag(ctx, loaded)
}

func (c Containers) present(ctx context.Context, image string) bool {
	return exec.CommandContext(ctx, c.Runtime, "image", "inspect", image).Run() == nil
}

func (c Containers) tag(ctx context.Context, loaded string) error {
	if out, err := exec.CommandContext(ctx, c.Runtime, "tag", loaded, c.Image).CombinedOutput(); err != nil {
		return fmt.Errorf("the loaded image %s could not be named %s: %s", loaded, c.Image, strings.TrimSpace(string(out)))
	}
	return nil
}

// imageLock is the file the servers of this machine hold while one fetches the runner image, named for the image.
func (c Containers) imageLock() string {
	name := strings.NewReplacer("/", "_", ":", "_").Replace(c.Image)
	return filepath.Join(os.TempDir(), "gusto-image-"+name+".lock")
}

// imageDownload has no limit on the whole request, because the image is large, and a server that never starts to answer is
// given up on. ctx bounds the rest.
var imageDownload = &http.Client{Transport: waitingAtMost(30 * time.Second)}

func waitingAtMost(d time.Duration) http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = d
	return t
}

// openImage streams the image's archive from where it is kept, with a wait that says whether the fetch itself went well.
func openImage(ctx context.Context, source string) (io.ReadCloser, func() error, error) {
	switch {
	case strings.HasPrefix(source, "s3://"):
		fetch := exec.CommandContext(ctx, awsCommand(), "s3", "cp", source, "-")
		var said bytes.Buffer
		fetch.Stderr = &said
		out, err := fetch.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}
		if err := fetch.Start(); err != nil {
			return nil, nil, err
		}
		return out, func() error {
			if err := fetch.Wait(); err != nil {
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(said.String()))
			}
			return nil
		}, nil
	case strings.HasPrefix(source, "https://"), strings.HasPrefix(source, "http://"):
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, nil, err
		}
		answer, err := imageDownload.Do(request)
		if err != nil {
			return nil, nil, err
		}
		if answer.StatusCode != http.StatusOK {
			answer.Body.Close()
			return nil, nil, fmt.Errorf("it answered %s", answer.Status)
		}
		return answer.Body, func() error { return nil }, nil
	}
	return nil, nil, errors.New("a source is an s3:// or http(s):// address")
}

// loadImage loads an image archive into the runtime and says the name it was loaded under.
func (c Containers) loadImage(ctx context.Context, archive io.Reader) (string, error) {
	load := exec.CommandContext(ctx, c.Runtime, "load")
	load.Stdin = archive
	out, err := load.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if name, found := strings.CutPrefix(strings.TrimSpace(line), "Loaded image: "); found {
			return strings.TrimSpace(name), nil
		}
	}
	return "", fmt.Errorf("the runtime did not say what it loaded: %s", strings.TrimSpace(string(out)))
}

// awsCommand is the AWS command line: on the PATH, or where a snap or an installer puts it, which a service's PATH may lack.
func awsCommand() string {
	if found, err := exec.LookPath("aws"); err == nil {
		return found
	}
	for _, place := range []string{"/snap/bin/aws", "/usr/local/bin/aws"} {
		if _, err := os.Stat(place); err == nil {
			return place
		}
	}
	return "aws"
}
