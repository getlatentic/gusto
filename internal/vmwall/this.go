package vmwall

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// This is the computer gusto runs on.
func This(release string, say io.Writer) Env {
	return Env{
		Platform: Platform{runtime.GOOS, runtime.GOARCH},
		Getenv:   os.Getenv,
		Exists:   func(path string) bool { _, err := os.Stat(path); return err == nil },
		Get:      OverHTTP,
		GetAsset: releaseAsset,
		Release:  release,
		Run:      run,
		Start:    startGateway,
		Say:      say,
	}
}

func run(ctx context.Context, env []string, program string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, program, args...)
	command.Env = env
	var out bytes.Buffer
	command.Stdout, command.Stderr = &out, &out
	if err := command.Run(); err != nil {
		return out.String(), fmt.Errorf("%s: %w: %s", strings.Join(append([]string{program}, args[:min(len(args), 2)]...), " "), err, strings.TrimSpace(out.String()))
	}
	return out.String(), nil
}

// gustoRepository is where gusto's releases are kept.
const gustoRepository = "getlatentic/gusto"

// releaseAsset downloads a file attached to a release of gusto. While the repository is private an address is answered with not
// found, and the GitHub command line, which the person has signed in to, is what can read it, as it is for installing gusto.
func releaseAsset(ctx context.Context, release, name string, dst io.Writer) error {
	var buffered bytes.Buffer
	err := OverHTTP(ctx, "https://github.com/"+gustoRepository+"/releases/download/"+release+"/"+name, &buffered)
	if err == nil {
		_, err = io.Copy(dst, &buffered)
		return err
	}
	gh, lookErr := exec.LookPath("gh")
	if lookErr != nil || !strings.Contains(err.Error(), "404") {
		return err
	}
	command := exec.CommandContext(ctx, gh, "release", "download", release, "-R", gustoRepository, "-p", name, "-O", "-")
	command.Stdout = dst
	var complaint bytes.Buffer
	command.Stderr = &complaint
	if err := command.Run(); err != nil {
		return fmt.Errorf("gh release download %s %s: %w: %s", release, name, err, strings.TrimSpace(complaint.String()))
	}
	return nil
}
