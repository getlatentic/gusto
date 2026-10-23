package runner

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/getlatentic/gusto/internal/acp"
	"github.com/getlatentic/gusto/pkg/job"
)

// hangUp is how long a harness is given to exit once the turn is over and its
// input is closed. An ACP server is meant to stop when its client goes; one
// that does not is stopped, since the turn it was asked for has ended.
const hangUp = 10 * time.Second

// converse holds one turn over the Agent Client Protocol. The harness's exit
// status is the turn's: an answered prompt is 0 whatever the teardown took, and
// a turn the harness failed or never answered is 1, with its reason in stderr.
// cwd is the directory the conversation works in, by the path the harness sees.
func converse(ctx context.Context, command *exec.Cmd, cwd string, spec job.Spec, servers []job.Server, auth string, record *Record) (int, error) {
	in, err := command.StdinPipe()
	if err != nil {
		return 0, err
	}
	out, err := command.StdoutPipe()
	if err != nil {
		return 0, err
	}
	errs, err := command.StderrPipe()
	if err != nil {
		return 0, err
	}
	if err := command.Start(); err != nil {
		return 0, err
	}
	exited := make(chan error, 1)
	go func() { exited <- keepErrors(errs, record, command) }()

	ended, failed := acp.Converse(in, out, acp.Turn{
		Cwd:     cwd,
		Brief:   spec.Brief,
		Session: spec.Remembered,
		Servers: servers,
		Auth:    auth,
	}, record.Out)
	record.Remembered = ended.Session
	in.Close()
	if err := hangUpOn(command, exited); err != nil {
		return 0, err
	}
	if failed == nil {
		return 0, nil
	}
	if ctx.Err() == nil {
		if err := record.Err("gusto: " + failed.Error()); err != nil {
			return 0, err
		}
	}
	return 1, nil
}

// keepErrors records the harness's stderr until it closes, then waits for the
// process, so the record is never written from two places at once.
func keepErrors(errs io.Reader, record *Record, command *exec.Cmd) error {
	lines := bufio.NewScanner(errs)
	lines.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var failure error
	for lines.Scan() {
		if err := record.Err(lines.Text()); err != nil && failure == nil {
			failure = err
		}
	}
	_ = command.Wait()
	return failure
}

func hangUpOn(command *exec.Cmd, exited <-chan error) error {
	select {
	case err := <-exited:
		return err
	case <-time.After(hangUp):
		_ = command.Process.Kill()
		return <-exited
	}
}

// resolved is the workspace by the path it really has. A harness can hold a
// session to the directory it began in, and a turn run without a container
// reaches the conversation's directory through a link that is new every turn.
func resolved(workspace string) string {
	if real, err := filepath.EvalSymlinks(workspace); err == nil {
		return real
	}
	return workspace
}
