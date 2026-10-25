package cell

// bringUp has a host start: a kept host that is stopped when there is one, since its disk is read already, and otherwise a new
// host, which is made a kept one while fewer are kept than Keep.
func (s *Scaler) bringUp(running []Instance) (string, error) {
	stopped, err := s.Launcher.Stopped()
	if err != nil {
		return "", err
	}
	for _, host := range stopped {
		if host.Stopped {
			return host.ID, s.Launcher.Start(host.ID)
		}
	}
	kept := len(stopped)
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
