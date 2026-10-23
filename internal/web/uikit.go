package web

import "net/http"

// uiKitPage draws every shared piece in every state beside the colours and type they are made of, so that a change to one is seen on all of them.
func (s *Server) uiKitPage(w http.ResponseWriter, r *http.Request) {
	s.drawConsole(w, "admin-ui-kit.html", "ui-kit", map[string]any{
		"Title":   productName + " — UI kit",
		"Surface": []string{"canvas", "wash", "sunken", "surface", "rail", "hover"},
		"Agents":  []string{"claude-code", "codex", "opencode", "hermes", "none"},
		"States":  []string{"passed", "finished", "live", "running", "queued", "failed", "interrupted", "idle"},
	})
}
