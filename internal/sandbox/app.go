package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// AppSocketDir is where an app's container finds the folder its socket is made in, and AppSocket the socket.
const (
	AppSocketDir = "/run/gusto/app"
	AppSocket    = AppSocketDir + "/app.sock"
	appFolder    = "/job/workspace"
)

// AppBox is the container a web app an agent built runs in. It has no network: the server reaches the app through a socket
// in SocketDir, a folder of the server's, and the app reaches nothing.
type AppBox struct {
	// Name is the container's name, by which it is stopped.
	Name string
	// Owner is whose app it is, and App its id, which label the container.
	Owner Owner
	App   string
	// Folder is the agent's folder the app is started in, and Home the person's kept home, or nothing.
	Folder, Home string
	// SocketDir is the folder on this machine the app's socket is made in.
	SocketDir string
	Command   string
	Port      int
}

// StartApp starts the app's container and leaves it running. A container left by an app of the same name is removed first.
func (c Containers) StartApp(ctx context.Context, box AppBox) error {
	if err := c.EnsureImage(ctx); err != nil {
		return err
	}
	if err := c.StopApp(ctx, box.Name); err != nil {
		return err
	}
	args, err := c.appArgv(box)
	if err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, c.Runtime, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("the app's container did not start: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// appArgv is the app container's command line. It is held as a turn's container is, and is kept once it ends so that what the
// app wrote can be read.
func (c Containers) appArgv(box AppBox) ([]string, error) {
	args := []string{"run", "-d", "--name", box.Name, "--network=none"}
	args = append(args, box.Owner.labels("")...)
	args = append(args, "--label", LabelPrefix+"app="+box.App)
	mounts := []Mount{{From: box.SocketDir, To: AppSocketDir}, {From: box.Folder, To: appFolder}}
	if box.Home != "" {
		mounts = append(mounts, Mount{From: box.Home, To: Home, Kept: true})
	}
	for _, mount := range mounts {
		absolute, err := filepath.Abs(mount.From)
		if err != nil {
			return nil, err
		}
		args = append(args, "-v", absolute+":"+mount.To)
		if mount.Kept {
			args = append(args, "-e", KeptHomeEnv+"=1")
		}
	}
	args = append(args, c.Limits()...)
	args = append(args, Hardening()...)
	return append(args, c.Image, "inside-app",
		"--socket", AppSocket, "--port", strconv.Itoa(box.Port), "--dir", appFolder, "--", box.Command), nil
}

// appStopGrace is how long an app's container is given to end its app and give its folder back before it is removed.
const appStopGrace = "15"

// StopApp stops an app's container, giving it the grace to end its app, then removes it. One that is not there is already
// stopped.
func (c Containers) StopApp(ctx context.Context, name string) error {
	_ = exec.CommandContext(ctx, c.Runtime, "stop", "-t", appStopGrace, name).Run()
	out, err := exec.CommandContext(ctx, c.Runtime, "rm", "-f", name).CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "no such container") {
		return fmt.Errorf("the app's container could not be removed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// AppLog is the last of what an app's container wrote, or nothing when it cannot be read.
func (c Containers) AppLog(ctx context.Context, name string) string {
	out, _ := exec.CommandContext(ctx, c.Runtime, "logs", "--tail", "40", name).CombinedOutput()
	return strings.TrimSpace(string(out))
}
