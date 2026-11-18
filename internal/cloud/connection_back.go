package cloud

import (
	"net/http"
	"strings"
)

// connectionBack hands a person coming back from signing in at an app on to their organization's server, which began the
// sign-in and put its organization first in the sign-in's state, as <org>.<random>. An app then has one address to send
// everybody back to, however organizations are reached. Only the address changes: the app's answer goes on as it came, and the
// organization's server checks the state and finishes the sign-in.
func (s *Server) connectionBack(w http.ResponseWriter, r *http.Request) {
	slug, _, named := strings.Cut(r.URL.Query().Get("state"), ".")
	if !named || ValidSlug(slug) != nil || s.Controller == nil {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, s.orgURL(slug)+"/connections/callback?"+r.URL.RawQuery, http.StatusSeeOther)
}
