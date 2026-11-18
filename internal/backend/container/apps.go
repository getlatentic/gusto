package container

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// sharedApps runs the web apps agents built, each in a container of its own with no network, beside the agent's folder and
// its person's kept home. This server reaches an app through a socket in a folder of its own that is mounted in.
type sharedApps struct {
	b *Backend
	// folder is an agent's folder, by its instance.
	folder func(instance string) string
	// sockets is the folder on this machine the apps' socket folders are made in.
	sockets string
}

// Apps runs the web apps agents built in containers of their own. It is nil when this server was given no agents' folders or no
// folder for their sockets, or when the runtime's containers are in a virtual machine that a socket made in one cannot be reached
// from, as they are on a Mac.
func (b *Backend) Apps() apps.Runner {
	if b.cfg.AgentFolder == nil || b.cfg.AppSockets == "" || runtime.GOOS != "linux" {
		return nil
	}
	return &sharedApps{b: b, folder: b.cfg.AgentFolder, sockets: b.cfg.AppSockets}
}

// appRunsOn says whether an app's container runs with the kept home of a computer.
func (b *Backend) appRunsOn(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), askRuntimeWithin)
	defer cancel()
	return b.cfg.Containers.AppRunsFor(ctx, sandbox.Owner{Org: b.cfg.Org, Person: id})
}

// askRuntimeWithin bounds asking the runtime which apps run.
const askRuntimeWithin = 30 * time.Second

// socketPathMost is the longest path a unix socket may have on Linux, which keeps 108 bytes for it with its ending zero.
const socketPathMost = 107

func orgOrLocal(org string) string {
	if org == "" {
		return "local"
	}
	return org
}

// name is the app's container's name. App ids are an organization's own, so the organization is in it.
func (a *sharedApps) name(app apps.App) string {
	return "gusto-app-" + orgOrLocal(a.b.cfg.Org) + "-" + app.ID
}

func (a *sharedApps) socketDir(app apps.App) string { return filepath.Join(a.sockets, app.ID) }

// socket is the app's socket as this server reaches it.
func (a *sharedApps) socket(app apps.App) string {
	return filepath.Join(a.socketDir(app), filepath.Base(sandbox.AppSocket))
}

// Start has the app's container running and the app answering, or says why it does not. A container that started an app
// that did not answer is removed, so a failed app leaves nothing running.
func (a *sharedApps) Start(ctx context.Context, app apps.App) (apps.App, error) {
	box, release, err := a.box(app)
	if err != nil {
		return apps.App{}, err
	}
	err = a.b.cfg.Containers.StartApp(ctx, box)
	release()
	if err != nil {
		return apps.App{}, err
	}
	app.Sandbox = box.Name
	app.Host = "localhost:" + strconv.Itoa(app.Port)
	app.Target = apps.SocketTarget(a.socket(app))
	if err := a.wait(ctx, app); err != nil {
		note := a.b.cfg.Containers.AppLog(context.Background(), box.Name)
		a.Stop(context.Background(), app)
		if note != "" {
			return apps.App{}, fmt.Errorf("%w. It wrote:\n%s", err, note)
		}
		return apps.App{}, err
	}
	return app, nil
}

// box is the container an app runs in: its agent's folder, its person's kept home when this server keeps homes, and a fresh
// folder for its socket that only this server's user may open. The home is not removed until release is called, once the
// container has started or failed to.
func (a *sharedApps) box(app apps.App) (box sandbox.AppBox, release func(), err error) {
	folder := a.folder(app.Instance)
	if _, err := os.Stat(folder); err != nil {
		return sandbox.AppBox{}, nil, errors.New("this agent has no folder yet: ask it something first")
	}
	if socket := a.socket(app); len(socket) > socketPathMost {
		return sandbox.AppBox{}, nil, fmt.Errorf("the app's socket, %s, would be too long a path: set GUSTO_APP_SOCKETS to a shorter folder", socket)
	}
	box = sandbox.AppBox{
		Name: a.name(app), App: app.ID, Folder: folder, SocketDir: a.socketDir(app), Command: app.Command, Port: app.Port,
		Owner: sandbox.Owner{Org: a.b.cfg.Org, Instance: app.Instance},
	}
	release = func() {}
	if app.Owner != "" {
		box.Owner.Person = computer.ID(a.b.cfg.Org, app.Team, app.Owner)
	}
	if a.b.homes != nil && app.Owner != "" {
		box.Home, release, err = a.b.homes.openForApp(box.Owner.Person)
		if err != nil {
			return sandbox.AppBox{}, nil, err
		}
	}
	if err := freshFolder(box.SocketDir); err != nil {
		release()
		return sandbox.AppBox{}, nil, err
	}
	return box, release, nil
}

// freshFolder is an empty folder that only this server's user may open.
func freshFolder(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o700)
}

// wait asks the app until it answers or the time is up. An app takes a moment to listen, and its socket is made only once
// its container has started.
func (a *sharedApps) wait(ctx context.Context, app apps.App) error {
	for {
		last := a.Answers(ctx, app)
		if last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the app did not answer in time: %v", last)
		case <-time.After(time.Second):
		}
	}
}

// Answers says whether the app answers a request through its socket with anything but a server error.
func (a *sharedApps) Answers(ctx context.Context, app apps.App) error {
	socket, onSocket := app.Socket()
	if !onSocket {
		return errors.New("this app has no socket to be reached through")
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+app.Host+"/", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode >= 500 {
		return fmt.Errorf("it answered %s", response.Status)
	}
	return nil
}

// Stop removes the app's container and its socket's folder. Each is tried whatever became of the other.
func (a *sharedApps) Stop(ctx context.Context, app apps.App) error {
	return errors.Join(a.b.cfg.Containers.StopApp(ctx, a.name(app)), os.RemoveAll(a.socketDir(app)))
}
