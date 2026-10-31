package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ImageSource is the setting that names where the runner image can be fetched from when this machine does not have it:
// an s3:// address read with the AWS command line, or an http(s):// one. A machine kept running across releases is
// handed a new image this way, since nothing else gives it one until it starts again.
const ImageSource = "GUSTO_IMAGE_SOURCE"

// imageFetchTime is the most fetching and loading the image may take: a gigabyte or so from the same region.
const imageFetchTime = 15 * time.Minute

// imageMu makes one fetch at a time in this process; a task that waits for it then finds the image there.
var imageMu sync.Mutex

// ensureImage makes the runner image present before a container is started from it, fetching and loading it when it is
// missing and a source is set. With no source, a missing image is left for the start to say so.
func ensureImage(ctx context.Context) error {
	source := os.Getenv(ImageSource)
	if source == "" {
		return nil
	}
	imageMu.Lock()
	defer imageMu.Unlock()
	if exec.CommandContext(ctx, Runtime(), "image", "inspect", Image()).Run() == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, imageFetchTime)
	defer cancel()
	archive, wait, err := openImage(ctx, source)
	if err != nil {
		return fmt.Errorf("the runner image %s is not here and could not be fetched from %s: %w", Image(), source, err)
	}
	defer archive.Close()
	loaded, err := loadImage(ctx, archive)
	if waitErr := wait(); err == nil {
		err = waitErr
	}
	if err != nil {
		return fmt.Errorf("the runner image %s could not be loaded from %s: %w", Image(), source, err)
	}
	if loaded != Image() {
		if out, err := exec.CommandContext(ctx, Runtime(), "tag", loaded, Image()).CombinedOutput(); err != nil {
			return fmt.Errorf("the loaded image %s could not be named %s: %s", loaded, Image(), strings.TrimSpace(string(out)))
		}
	}
	return nil
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
		answer, err := http.DefaultClient.Do(request)
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
func loadImage(ctx context.Context, archive io.Reader) (string, error) {
	load := exec.CommandContext(ctx, Runtime(), "load")
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
