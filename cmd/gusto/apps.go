package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/openshell"
)

// sharedApps runs the web apps that agents built in the computers of the people whose agents they
// are. An app is a process in the computer, given an address on the gateway that answers
// only this server, and this server passes on what a signed-in member of the organization asks it.
type sharedApps struct {
	cfg      openshell.Config
	registry *computer.Registry
	store    *apps.Store
	client   *http.Client
}

func newSharedApps(cfg openshell.Config, registry *computer.Registry, store *apps.Store) *sharedApps {
	return &sharedApps{cfg: cfg, registry: registry, store: store, client: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// serviceName is what the gateway calls an app's address.
func serviceName(app apps.App) string { return "a" + app.ID }

// Start has the app running and answering, or says why it does not. What it started is stopped
// when it did not answer, so a failed app leaves nothing running.
func (a *sharedApps) Start(ctx context.Context, app apps.App) (apps.App, error) {
	held, err := a.registry.Of(app.Owner)
	if err != nil {
		return apps.App{}, errors.New("the person this agent belongs to has no workspace to run it in")
	}
	if err := openshell.Wake(ctx, a.cfg, held.Sandbox); err != nil {
		return apps.App{}, err
	}
	app.Sandbox = held.Sandbox
	if err := openshell.StartApp(ctx, a.cfg, held.Sandbox, app.Instance, app.ID, app.Command, app.Port); err != nil {
		return apps.App{}, err
	}
	address, err := a.wait(ctx, app)
	if err != nil {
		note := openshell.AppLog(context.Background(), a.cfg, held.Sandbox, app.ID)
		a.Stop(context.Background(), app)
		if note != "" {
			return apps.App{}, fmt.Errorf("%w. It wrote:\n%s", err, note)
		}
		return apps.App{}, err
	}
	app.Host, app.Target = address.Host, address.Target
	return app, nil
}

// wait exposes the port and asks it until something answers. An app takes a moment to listen, and the
// gateway may not expose a port nothing listens on yet, so both are asked again until the time is up.
func (a *sharedApps) wait(ctx context.Context, app apps.App) (address openshell.Address, err error) {
	var exposed bool
	last := errors.New("nothing was tried")
	for {
		if !exposed {
			if address, last = openshell.ExposeApp(ctx, a.cfg, app.Sandbox, app.Port, serviceName(app)); last == nil {
				exposed = true
			}
		}
		if exposed {
			if last = a.answers(ctx, app, address); last == nil {
				return address, nil
			}
		}
		select {
		case <-ctx.Done():
			return openshell.Address{}, fmt.Errorf("the app did not answer in time: %v", last)
		case <-time.After(time.Second):
		}
	}
}

// answers is whether the app answers a request with anything but a server error.
func (a *sharedApps) answers(ctx context.Context, app apps.App, address openshell.Address) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address.Target+"/", nil)
	if err != nil {
		return err
	}
	request.Host = address.Host
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode >= 500 {
		return fmt.Errorf("it answered %s", response.Status)
	}
	if app.IsPreview() && response.Header.Get(apps.PreviewHeader) == "" {
		return errors.New("something else answers on that port, not the preview")
	}
	return nil
}

// Stop takes the app's address away and stops its processes. Each is tried whatever became of the other.
func (a *sharedApps) Stop(ctx context.Context, app apps.App) error {
	if app.Sandbox == "" {
		return nil
	}
	return errors.Join(
		openshell.UnexposeApp(ctx, a.cfg, app.Sandbox, serviceName(app)),
		openshell.StopApp(ctx, a.cfg, app.Sandbox, app.ID),
	)
}

// serving is whether a person's computer has an app that people use, which is why it is not stopped for being idle.
func (a *sharedApps) serving(person string) bool {
	all, err := a.store.All()
	if err != nil {
		return false
	}
	for _, app := range all {
		if strings.EqualFold(app.Owner, person) && (app.State == apps.Live || app.State == apps.Starting) {
			return true
		}
	}
	return false
}
