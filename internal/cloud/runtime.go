package cloud

import (
	"context"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// State is where an organization's server is.
type State string

const (
	Provisioning State = "provisioning"
	Starting     State = "starting"
	Ready        State = "ready"
	// Waiting is an organization no cell has room for yet, which is tried again in a few seconds.
	Waiting State = "waiting"
	Failed  State = "failed"
)

// Runtime is what the front door knows of an organization's server: which cell it
// is on, the two ports it was given, and how it is doing. It is written by the
// controller alone, beside the organization's record and apart from it.
type Runtime struct {
	State State  `json:"state"`
	Cell  string `json:"cell,omitempty"`
	// Port is where the organization's server listens, and ProxyPort where its
	// proxy for models does, both on the cell's own loopback.
	Port      int       `json:"port,omitempty"`
	ProxyPort int       `json:"proxyPort,omitempty"`
	Error     string    `json:"error,omitempty"`
	Restarts  int       `json:"restarts,omitempty"`
	Updated   time.Time `json:"updated"`
}

var runtimeFields = []string{"slug", "state", "cell", "port", "proxy_port", "error", "restarts", "updated"}

var putRuntime = dbkit.Upsert{Table: "runtime", Columns: runtimeFields, Conflict: []string{"slug"}, Update: runtimeFields[1:]}

func scanRuntime(r rowScanner) (Runtime, error) {
	var rt Runtime
	var state string
	var updated dbkit.Time
	if err := r.Scan(&state, &rt.Cell, &rt.Port, &rt.ProxyPort, &rt.Error, &rt.Restarts, &updated); err != nil {
		return Runtime{}, err
	}
	rt.State, rt.Updated = State(state), updated.Time
	return rt, nil
}

// Runtime is the state of an organization's server. One never provisioned has none.
func (s *Store) Runtime(slug string) (Runtime, error) {
	rt, _, err := one(context.Background(), s.db, scanRuntime,
		`SELECT state, cell, port, proxy_port, error, restarts, updated FROM runtime WHERE slug = ?`, slug)
	return rt, err
}

// SetRuntime keeps the state of an organization's server. Two organizations on one cell cannot be given
// the same port, even by two front doors.
func (s *Store) SetRuntime(slug string, rt Runtime) error {
	rt.Updated = s.now().UTC().Truncate(time.Second)
	_, err := s.db.ExecContext(context.Background(), putRuntime.SQL(1),
		slug, string(rt.State), rt.Cell, rt.Port, rt.ProxyPort, rt.Error, rt.Restarts, s.stamp(rt.Updated))
	return err
}
