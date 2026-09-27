package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"

	"github.com/getlatentic/haas/internal/job"
)

// A pane is one size. Every watcher's terminal is drawn at it, so what one
// sees is what another sees, and what was recorded replays as it was.
const (
	PaneCols = 120
	PaneRows = 36
)

// terminal runs a harness in its own interface, on a terminal the runner
// holds, for a person to watch and take over. What reaches the runner's input
// is typed into it, and what it draws goes to the runner's output and into
// out/pane.cast. The harness's exit is the run's.
func terminal(ctx context.Context, command *exec.Cmd, dir job.Dir, input io.Reader, output io.Writer) (int, error) {
	if command.Env == nil {
		command.Env = os.Environ()
	}
	command.Env = append(command.Env, "TERM=xterm-256color", "COLORTERM=truecolor")
	screen, err := pty.StartWithSize(command, &pty.Winsize{Rows: PaneRows, Cols: PaneCols})
	if err != nil {
		return 0, err
	}
	defer screen.Close()
	record, err := openCast(dir.OutFile("pane.cast"), PaneCols, PaneRows)
	if err != nil {
		return 0, err
	}
	go func() { _, _ = io.Copy(io.MultiWriter(screen, recorded{record, "i"}), input) }()
	// Reading a terminal whose program has exited fails with EIO, which is how
	// its end arrives rather than an error.
	_, _ = io.Copy(io.MultiWriter(output, recorded{record, "o"}), screen)
	waited := command.Wait()
	if err := record.Close(); err != nil {
		return 0, err
	}
	var exit *exec.ExitError
	if waited != nil && !errors.As(waited, &exit) && ctx.Err() == nil {
		return 0, waited
	}
	return command.ProcessState.ExitCode(), nil
}

// fromTheBox is the runner's own input and output, which the container's are.
func fromTheBox() (io.Reader, io.Writer) { return os.Stdin, os.Stdout }
