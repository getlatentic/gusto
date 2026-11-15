package cloud

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// State is where an organization's server is.
type State string

const (
	Provisioning State = "provisioning"
	Starting     State = "starting"
	Ready        State = "ready"
	// Waiting is an organization no node has room for yet, which is tried again in a few seconds.
	Waiting State = "waiting"
	Failed  State = "failed"
)

// Runtime is what gusto cloud knows of an organization's server: which node it
// is on, the two ports it was given, and how it is doing. It is written by the
// controller alone, beside the organization's record and apart from it.
type Runtime struct {
	State State  `json:"state"`
	Node  string `json:"cell,omitempty"`
	// Port is where the organization's server listens, and ProxyPort where its
	// proxy for models does, both on the node's own loopback.
	Port      int       `json:"port,omitempty"`
	ProxyPort int       `json:"proxyPort,omitempty"`
	Error     string    `json:"error,omitempty"`
	Restarts  int       `json:"restarts,omitempty"`
	Updated   time.Time `json:"updated"`
	// Command is what the server was started with. A gusto cloud server whose command is another restarts it, which is how an
	// organization's server comes to gusto cloud's release.
	Command string `json:"command,omitempty"`
}

var runtimeFields = []string{"slug", "state", "cell", "port", "proxy_port", "error", "restarts", "updated", "command"}

var putRuntime = dbkit.Upsert{Table: "runtime", Columns: runtimeFields, Conflict: []string{"slug"}, Update: runtimeFields[1:]}

func scanRuntime(r rowScanner) (Runtime, error) {
	var rt Runtime
	var state string
	var updated dbkit.Time
	if err := r.Scan(&state, &rt.Node, &rt.Port, &rt.ProxyPort, &rt.Error, &rt.Restarts, &updated, &rt.Command); err != nil {
		return Runtime{}, err
	}
	rt.State, rt.Updated = State(state), updated.Time
	return rt, nil
}

// Runtime is the state of an organization's server. One never provisioned has none.
func (s *Store) Runtime(ctx context.Context, slug string) (Runtime, error) {
	rt, _, err := one(ctx, s.db, scanRuntime,
		`SELECT state, cell, port, proxy_port, error, restarts, updated, command FROM runtime WHERE slug = ?`, slug)
	return rt, err
}

// SetRuntime keeps the state of an organization's server. Two organizations on one node cannot be given
// the same port, even by two gusto cloud servers. An organization that is gone, as an ended demo is, has no state to keep and is
// errNoOrg.
func (s *Store) SetRuntime(ctx context.Context, slug string, rt Runtime) error {
	rt.Updated = s.now().UTC().Truncate(time.Second)
	return s.db.InTx(ctx, func(tx dbkit.Tx) error { return s.writeRuntime(ctx, tx, slug, rt) })
}

// writeRuntime writes the state under the organization's lock, the one EndDemo deletes the organization under, so a write is
// either before the deletion and deleted with it, or after it and refused, and never fails on a record that is not there.
func (s *Store) writeRuntime(ctx context.Context, tx dbkit.Tx, slug string, rt Runtime) error {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return err
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM orgs WHERE slug = ?`, slug).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return errNoOrg
	} else if err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, putRuntime.SQL(1),
		slug, string(rt.State), rt.Node, rt.Port, rt.ProxyPort, rt.Error, rt.Restarts, s.stamp(rt.Updated), rt.Command)
	return err
}
