package cloud

import "net/http"

// privacy says what the front door collects, keeps and passes on.
func (s *Server) privacy(w http.ResponseWriter, _ *http.Request) {
	s.render(w, http.StatusOK, "privacy", view{Title: "Privacy notice · " + siteName, DemoLeft: s.Store.set.Demo.Length})
}

// terms says what the service is for and what may not be done with it.
func (s *Server) terms(w http.ResponseWriter, _ *http.Request) {
	s.render(w, http.StatusOK, "terms", view{Title: "Terms of use · " + siteName, DemoLeft: s.Store.set.Demo.Length})
}
