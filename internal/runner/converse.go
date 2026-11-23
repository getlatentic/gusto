package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/getlatentic/gusto/internal/acp"
	"github.com/getlatentic/gusto/pkg/task"
)

// hangUp is how long a harness is given to exit once the turn is over and its
// input is closed. An ACP server is meant to stop when its client goes; one
// that does not is stopped, since the turn it was asked for has ended.
const hangUp = 10 * time.Second

// cancelGrace is how long an ACP agent is given to end a turn it was asked to cancel, before it is killed. A container's stop
// grace is longer, so the agent is asked first and killed only when it does not answer.
const cancelGrace = 15 * time.Second

// outliving is a context for an ACP agent's process: it ends grace after ctx does, or when its cancel is called, so a stopped
// turn is cancelled by the agent before the agent is killed.
func outliving(ctx context.Context, grace time.Duration) (context.Context, context.CancelFunc) {
	life, end := context.WithCancel(context.WithoutCancel(ctx))
	go func() {
		select {
		case <-ctx.Done():
		case <-life.Done():
			return
		}
		select {
		case <-time.After(grace):
			end()
		case <-life.Done():
		}
	}()
	return life, end
}

// converse holds one turn over the Agent Client Protocol. The harness's exit
// status is the turn's: an answered prompt is 0 whatever the teardown took, and
// a turn the harness failed, never answered, or ended for another reason than
// being done is 1, with its reason in stderr. When ctx ends the agent is asked
// to cancel the turn; the command must be started with a context that outlives
// ctx (outliving), so it has the time to.
// cwd is the directory the conversation works in, by the path the harness sees.
func converse(ctx context.Context, command *exec.Cmd, cwd string, spec task.Spec, servers []task.Server, auth string, record *Recorder) (int, error) {
	in, out, errs, err := started(command)
	if err != nil {
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
		Stop:    ctx.Done(),
	}, record.Out)
	record.Remembered = ended.Session
	in.Close()
	if err := hangUpOn(command, exited); err != nil {
		return 0, err
	}
	if failed == nil && !ended.Answered() {
		failed = fmt.Errorf("the agent ended the turn without answering it: %s", ended.Stop)
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

// started starts the harness with its standard input, output and errors piped to the runner.
func started(command *exec.Cmd) (in io.WriteCloser, out, errs io.ReadCloser, err error) {
	if in, err = command.StdinPipe(); err != nil {
		return nil, nil, nil, err
	}
	if out, err = command.StdoutPipe(); err != nil {
		return nil, nil, nil, err
	}
	if errs, err = command.StderrPipe(); err != nil {
		return nil, nil, nil, err
	}
	return in, out, errs, command.Start()
}

// keepErrors records the harness's stderr until it closes, then waits for the
// process, so the record is never written from two places at once.
func keepErrors(errs io.Reader, record *Recorder, command *exec.Cmd) error {
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

// resolved is the working folder by the path it really has. A harness can hold a
// session to the directory it began in, and a turn run without a container
// reaches the conversation's directory through a link that is new every turn.
func resolved(workingFolder string) string {
	if real, err := filepath.EvalSymlinks(workingFolder); err == nil {
		return real
	}
	return workingFolder
}
