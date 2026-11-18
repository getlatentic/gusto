package container

import "errors"

// ErrHomeInApp is a home an app a person shared runs on, which is not removed under it.
var ErrHomeInApp = errors.New("an app you shared runs on this home")

// openForApp is the home an app's container is about to mount, made when it is not there. The home is not removed until release
// is called, once the container runs and the runtime can say so.
func (h *homes) openForApp(id string) (dir string, release func(), err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.leaving(id); err != nil {
		return "", nil, err
	}
	if err := mkdirHome(h.dir(id)); err != nil {
		return "", nil, err
	}
	h.starting[id]++
	release = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.starting[id]--; h.starting[id] <= 0 {
			delete(h.starting, id)
		}
	}
	return h.dir(id), release, nil
}

// noAppOn refuses removing a home an app's container runs on. It is asked once the home is marked as being removed, so no app
// starts on it after the runtime has answered.
func (h *homes) noAppOn(id string) error {
	if h.appsOn == nil {
		return nil
	}
	running, err := h.appsOn(id)
	if err != nil {
		return err
	}
	if running {
		return ErrHomeInApp
	}
	return nil
}
