package cell

import (
	"log/slog"
	"sort"
)

// bringUp has a host start: the kept host that is stopped when there is one, since its disk is read already, and otherwise a new
// host, which is made a kept one while fewer are kept than Keep. A host that is stopped and not kept is never started: the sweep
// removes it.
func (s *Scaler) bringUp(running []Instance) (string, error) {
	stopped, err := s.Launcher.Stopped()
	if err != nil {
		return "", err
	}
	kept := 0
	for _, host := range stopped {
		if !host.Kept {
			continue
		}
		if host.Stopped {
			return host.ID, s.Launcher.Start(host.ID)
		}
		kept++
	}
	for _, host := range running {
		if host.Kept {
			kept++
		}
	}
	return s.Launcher.Launch(kept < s.Keep)
}

// putAway stops a kept host, so it can be started again, and removes any other.
func (s *Scaler) putAway(id string, kept bool) error {
	if kept {
		return s.Launcher.Stop(id)
	}
	return s.Launcher.Terminate(id)
}

// sweep removes the stopped hosts that are not the kept ones, which nothing would start: a host that is not kept and powered itself
// off, and kept hosts beyond Keep. A kept host that powered itself off, as a host does at its twelfth hour, is left stopped to be
// started again, and its disk is lowered to the baseline, which the scaler does when it stops a host but a host that stopped itself
// has not had. A host still stopping is looked at again at the next tick.
func (s *Scaler) sweep(running []Instance) {
	stopped, err := s.Launcher.Stopped()
	if err != nil {
		slog.Error("the stopped hosts could not be listed", "error", err)
		return
	}
	sort.Slice(stopped, func(i, j int) bool { return stopped[i].ID < stopped[j].ID })
	room := s.Keep
	for _, host := range running {
		if host.Kept {
			room--
		}
	}
	lowered := map[string]bool{}
	for _, host := range stopped {
		if host.Kept && room > 0 {
			room--
			if host.Stopped {
				lowered[host.ID] = s.lowered[host.ID] || s.lower(host.ID)
			}
			continue
		}
		if host.Stopped {
			s.remove(host.ID)
		}
	}
	s.lowered = lowered
}

func (s *Scaler) lower(id string) bool {
	if err := s.Launcher.LowerDisk(id); err != nil {
		slog.Warn("a stopped host's disk could not be lowered to the baseline", "host", id, "error", err)
		return false
	}
	return true
}

func (s *Scaler) remove(id string) {
	slog.Info("a stopped host is not the kept one, so it is removed", "host", id)
	if err := s.Launcher.Terminate(id); err != nil {
		slog.Error("a stopped host could not be removed", "host", id, "error", err)
	}
}
