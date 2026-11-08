package web

import "net/http"

// uiKitPage draws every shared piece in every state beside the colours and type they are made of, so that a change to one is seen on all of them.
func (s *Server) uiKitPage(w http.ResponseWriter, r *http.Request) {
	s.drawConsole(w, "admin-ui-kit.html", "ui-kit", map[string]any{
		"Title":   productName + " — UI kit",
		"Surface": []string{"canvas", "wash", "sunken", "surface", "rail", "hover"},
		"Agents":  []string{"claude", "codex", "opencode", "hermes", "pi", "dsh", "none"},
		"States":  []string{"passed", "finished", "live", "running", "queued", "failed", "interrupted", "idle"},
		"Tabs":    []tab{{Label: "Activity", Href: "#kit-surfaces", On: true}, {Label: "Task checks", Href: "#kit-surfaces"}, {Label: "Keeping", Href: "#kit-surfaces"}},
	})
}
