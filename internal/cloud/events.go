package cloud

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/getlatentic/gusto/internal/inbox"
)

// record tells the organisation's server what an admin just did to its people, so
// that the server writes it in its change record. The front door does not write the
// record itself: the server is its one writer, and two processes appending to one
// chained file would fork it. The change has been made by the time this is called, so
// a failure is said loudly and does not undo it.
func (s *Server) record(r *http.Request, by Person, kind string, what map[string]string) {
	item := inbox.Item{At: time.Now().UTC(), Who: by.Email, Role: string(by.Role), From: s.source(r), Kind: kind, What: what}
	if err := inbox.Drop(s.Store.InboxDir(by.Org), item); err != nil {
		slog.Error("a change to an organisation's people was not passed to its server", "org", by.Org, "kind", kind, "error", err)
	}
}
