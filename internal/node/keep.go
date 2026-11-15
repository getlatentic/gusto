package node

import (
	"errors"
	"io/fs"
	"log/slog"
	"slices"
	"sort"
)

// bringUp has a node start: the kept node that is stopped when there is one, since its disk is read already, and otherwise a new
// node, which is made a kept one while fewer are kept than Keep. A node that is stopped and not kept is never started: the sweep
// removes it. Nor is a kept node made from an image that is no longer the current one: it would run the whole set-up on its way
// up, and what that leaves on its disk is not what a node of the current image has, so it is removed and a new one made.
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
			s.Pool.setRetiring(NodeOf(host.ID), false)
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

// putAway stops a kept node, so it can be started again, and removes any other.
func (s *Scaler) putAway(id string, kept bool) error {
	if kept {
		return s.Launcher.Stop(id)
	}
	return s.Launcher.Terminate(id)
}

// sweep removes the stopped nodes that are not the kept ones, which nothing would start: a node that is not kept and powered itself
// off, and kept nodes beyond Keep. A kept node that powered itself off, as a node does at its twelfth hour, is left stopped to be
// started again, and its disk is lowered to the baseline, which the scaler does when it stops a node but a node that stopped itself
// has not had. A node still stopping is looked at again at the next tick. A stopping or stopped node has its registration taken
// away, which one that beat again on its way down left. It says which nodes are stopping or stopped, or nil when that is not known.
func (s *Scaler) sweep(running []Instance) []Instance {
	stopped, err := s.Launcher.Stopped()
	if err != nil {
		slog.Error("the stopped hosts could not be listed", "error", err)
		return nil
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
		s.unregister(host.ID)
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
	return stopped
}

// forgetRetired lets go of the mark on a node that is gone: no longer listed, running or stopped, and not registered. A node
// still stopping, or stopped to be started again, stays marked until bringUp starts it.
func (s *Scaler) forgetRetired(running, stopped []Instance) {
	listed := map[string]bool{}
	for _, host := range append(running, stopped...) {
		listed[NodeOf(host.ID)] = true
	}
	s.Pool.forgetRetiring(func(name string) bool {
		_, _, err := Registered(s.Pool.Dir, name, s.clock())
		return !listed[name] && errors.Is(err, fs.ErrNotExist)
	})
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

// currentImage is the image new nodes are made from, read only when a kept node is stopped and so might be started again. It is
// empty when it cannot be read, and then no node is taken to be out of date.
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

// stale says a node was made from an image other than the current one.
func stale(host Instance, current string) bool {
	return current != "" && host.Image != "" && host.Image != current
}
