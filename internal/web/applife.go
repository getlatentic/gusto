package web

import (
	"context"
	"log/slog"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
)

// startApp waits for the runner to have the app answering, and records how that went.
func (s *Server) startApp(app apps.App) {
	ctx, cancel := context.WithTimeout(context.Background(), appStart)
	defer cancel()
	started, err := s.runner.Start(ctx, app)
	if err != nil {
		note := err.Error()
		if len(note) > 1500 {
			note = note[len(note)-1500:]
		}
		after, _ := s.apps.Change(app.ID, func(a *apps.App) {
			if a.State == apps.Starting {
				a.State, a.Note = apps.Failed, note
			}
		})
		if after.State == apps.Removing {
			s.apps.Delete(app.ID)
		}
		return
	}
	after, err := s.apps.Change(app.ID, func(a *apps.App) {
		if a.State == apps.Starting {
			a.State, a.Sandbox, a.Host, a.Target, a.Note = apps.Live, started.Sandbox, started.Host, started.Target, ""
		}
	})
	if err != nil || after.State != apps.Live {
		// Removed while it started: what was started is stopped, so nothing runs that the page no longer shows.
		stop, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.runner.Stop(stop, started)
		if err == nil {
			s.apps.Delete(app.ID)
		}
	}
}

// stopApp stops an app's processes, gives its address back, and forgets it, whatever its state. An app that would not stop is
// still forgotten: the computer it ran in is what limits it. One still starting is marked removing, and its start, which may
// not yet have started anything, stops it and forgets it when it ends.
func (s *Server) stopApp(app apps.App) {
	if app.Folder != "" || s.runner == nil {
		s.apps.Delete(app.ID)
		return
	}
	marked, err := s.apps.Change(app.ID, func(a *apps.App) {
		if a.State == apps.Starting {
			a.State = apps.Removing
		}
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.runner.Stop(ctx, marked)
	if marked.State != apps.Removing {
		s.apps.Delete(app.ID)
	}
}

// checkApp asks an app a request could not reach whether it answers, a few times a second apart, and only when it never does
// stops it and says so: one reset connection is not an app that stopped.
func (s *Server) checkApp(id string) {
	if _, busy := s.checking.LoadOrStore(id, true); busy {
		return
	}
	s.publishing.Add(1)
	go func() {
		defer s.publishing.Done()
		defer s.checking.Delete(id)
		for range appChecks {
			app, err := s.apps.Get(id)
			if err != nil || app.State != apps.Live {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = s.runner.Answers(ctx, app)
			cancel()
			if err == nil {
				return
			}
			time.Sleep(time.Second)
		}
		app, err := s.apps.Change(id, func(a *apps.App) {
			if a.State == apps.Live {
				a.State, a.Note = apps.Stopped, "it stopped answering"
			}
		})
		if err != nil || app.State != apps.Stopped {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.runner.Stop(ctx, app)
	}()
}

// ReconcileApps finishes what a restart cut off, before any app is started: an app left starting is stopped and marked
// failed, one left being removed is stopped and forgotten, and a live app that no longer answers is started again where it was.
func (s *Server) ReconcileApps() {
	if s.apps == nil || s.runner == nil {
		return
	}
	all, err := s.apps.All()
	if err != nil {
		slog.Error("the shared apps could not be read to finish what a restart cut off", "error", err)
		return
	}
	for _, app := range all {
		switch {
		case app.State == apps.Live && app.Folder == "":
			s.restartApp(app)
		case app.State == apps.Starting || app.State == apps.Removing:
			s.finishCutOff(app)
		}
	}
}

// finishCutOff stops an app a restart left starting or being removed, and marks it failed or forgets it.
func (s *Server) finishCutOff(app apps.App) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	s.runner.Stop(ctx, app)
	cancel()
	if app.State == apps.Removing {
		s.apps.Delete(app.ID)
		return
	}
	s.apps.Change(app.ID, func(a *apps.App) {
		if a.State == apps.Starting {
			a.State, a.Note = apps.Failed, "the server restarted while it was starting"
		}
	})
}

// restartApp starts a live app again, keeping its address, when it does not answer: an app in a container ends with the
// server that started it. While it starts nobody checks it, and when it will not start it is marked stopped.
func (s *Server) restartApp(app apps.App) {
	if _, busy := s.checking.LoadOrStore(app.ID, true); busy {
		return
	}
	s.publishing.Add(1)
	go func() {
		defer s.publishing.Done()
		defer s.checking.Delete(app.ID)
		ctx, cancel := context.WithTimeout(context.Background(), appStart)
		defer cancel()
		if s.runner.Answers(ctx, app) == nil {
			return
		}
		started, startErr := s.runner.Start(ctx, app)
		after, err := s.apps.Change(app.ID, func(a *apps.App) {
			switch {
			case a.State != apps.Live:
			case startErr != nil:
				a.State, a.Note = apps.Stopped, "it did not start again after the server restarted"
			default:
				a.Sandbox, a.Host, a.Target = started.Sandbox, started.Host, started.Target
			}
		})
		if startErr == nil && (err != nil || after.State != apps.Live) {
			// Removed while it started again: what was started is stopped.
			stop, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			s.runner.Stop(stop, started)
		}
	}()
}
