package cell

import (
	"log/slog"
	"slices"
	"sort"
)

// bringUp has a host start: the kept host that is stopped when there is one, since its disk is read already, and otherwise a new
// host, which is made a kept one while fewer are kept than Keep. A host that is stopped and not kept is never started: the sweep
// removes it. Nor is a kept host made from an image that is no longer the current one: it would run the whole set-up on its way
// up, and what that leaves on its disk is not what a host of the current image has, so it is removed and a new one made.
func (s *Scaler) bringUp(running []Instance) (string, error) {
	stopped, err := s.Launcher.Stopped()
	if err != nil {
		return "", err
	}
	current := s.currentImage(stopped)
	kept := 0
	for _, host := range stopped {
		if !host.Kept {
			continue
		}
		if stale(host, current) {
			if host.Stopped {
				s.remove(host.ID)
			}
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
	current := s.currentImage(stopped)
	lowered := map[string]bool{}
	for _, host := range stopped {
		if host.Kept && room > 0 && !stale(host, current) {
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

// currentImage is the image new hosts are made from, read only when a kept host is stopped and so might be started again. It is
// empty when it cannot be read, and then no host is taken to be out of date.
func (s *Scaler) currentImage(stopped []Instance) string {
	if !slices.ContainsFunc(stopped, func(host Instance) bool { return host.Kept }) {
		return ""
	}
	image, err := s.Launcher.CurrentImage()
	if err != nil {
		slog.Warn("the image new hosts are made from could not be read", "error", err)
		return ""
	}
	return image
}

// stale says a host was made from an image other than the current one.
func stale(host Instance, current string) bool {
	return current != "" && host.Image != "" && host.Image != current
}
