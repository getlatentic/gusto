package runner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/pkg/proxy"
)

// App is a web app an agent built, run in a box of its own with no network. The server outside reaches it through Socket,
// a unix socket on a folder both share, and each connection there is passed to the app on the box's loopback.
type App struct {
	// Command is the one line that starts the app, run by the shell in Folder.
	Command string
	// Port is the loopback port the app listens on, which it is told as PORT.
	Port int
	// Folder is the agent's folder, which the app is started in.
	Folder string
	// Socket is where the server reaches the app.
	Socket string
}

// appStop is how long an app is given to end once it is asked to.
const appStop = 10 * time.Second

// ServeApp runs the app as the agent until it ends or ctx does, passing every connection to Socket on to it, and returns the
// app's exit code.
func ServeApp(ctx context.Context, app App, place Place) (int, error) {
	agent := agentIn(place)
	if err := agent.giveApp(app.Folder); err != nil {
		return 0, err
	}
	defer agent.takeBackApp(app)
	listener, err := listenFor(app.Socket)
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	go passAll(listener, "127.0.0.1:"+strconv.Itoa(app.Port))
	command := exec.CommandContext(ctx, "sh", "-c", app.Command)
	command.Dir = app.Folder
	command.Env = append(os.Environ(), "PORT="+strconv.Itoa(app.Port), "HOST=127.0.0.1")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	command.Cancel = func() error {
		time.AfterFunc(appStop, agent.endAll)
		return agent.terminate(command.Process)
	}
	command.WaitDelay = appStop
	agent.runAs(command)
	var exit *exec.ExitError
	err = command.Run()
	switch {
	case ctx.Err() != nil:
		// Stopped, as it was asked to be.
		return 0, nil
	case err != nil && !asExit(err, &exit):
		return 0, err
	}
	return command.ProcessState.ExitCode(), nil
}

// giveApp gives the agent its folder, and the kept home's own folder, so the app can write where the agent wrote.
func (a agent) giveApp(folder string) error {
	if !a.present {
		return nil
	}
	if err := a.own(folder); err != nil {
		return err
	}
	if !a.keptHome {
		return nil
	}
	return os.Lchown(os.Getenv("HOME"), a.uid, a.gid)
}

// takeBackApp gives the folder back, once the app has ended, to whoever owns the folder its socket is in: the server outside,
// which otherwise could not remove what the app wrote. It does what it can, as a turn's does.
func (a agent) takeBackApp(app App) {
	if !a.present {
		return
	}
	if owner, owned := ownerOfPath(filepath.Dir(app.Socket)); owned {
		_ = chownTree(app.Folder, owner.uid, owner.gid)
	}
}

// terminate asks the app to end. The box's root may not signal the agent's processes, as it holds no right to, so the agent
// asks every process of its own to end, which is the app and whatever it started.
func (a agent) terminate(app *os.Process) error {
	if !a.present {
		return app.Signal(syscall.SIGTERM)
	}
	ask := exec.Command("sh", "-c", "kill -TERM -1")
	a.runAs(ask)
	return ask.Run()
}

// listenFor opens the socket the server reaches the app through. The box's runner is root and the server outside may not be,
// so anyone may connect to it: the folder it is in is the server's own, which nobody else can open.
func listenFor(socket string) (net.Listener, error) {
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("the app's socket could not open at %s: %w", socket, err)
	}
	if err := os.Chmod(socket, 0o666); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

// passAll hands every connection the listener accepts to the app, until the listener is closed.
func passAll(listener net.Listener, app string) {
	for {
		near, err := listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			continue
		}
		go func() {
			far, err := net.DialTimeout("tcp", app, 5*time.Second)
			if err != nil {
				near.Close()
				return
			}
			proxy.Splice(near, far)
		}()
	}
}
