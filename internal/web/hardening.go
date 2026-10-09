package web

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// maxBody is the most a request may send. A form or a JSON job is a few
// kilobytes; the largest thing posted is a skill's text.
const maxBody = 4 << 20

// secure gives every response the headers that keep a page from being framed,
// sniffed or scripted from elsewhere, and keeps a request from sending more than
// a page ever does. The content security policy allows this server's own
// scripts and the inline ones that carry the request's nonce, and nothing else,
// so markup that got into a page could not run.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := randomValue()
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'nonce-"+nonce+"'; "+
			"style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; "+
			"form-action 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		if overTLS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		// What is behind the sign-in is not kept by a browser or a proxy. Only
		// the vendored assets are, and they set their own.
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			h.Set("Cache-Control", "no-store")
		}
		if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, bodyLimit(r))
		}
		next.ServeHTTP(&marked{ResponseWriter: w, nonce: nonce, admin: s.admin(r), page: r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html")}, r)
	})
}

// marked carries what a page needs to know of its request: the nonce its inline
// scripts run with, and whether an admin asked, which is what shows the link to
// the console on every page.
type marked struct {
	http.ResponseWriter
	nonce string
	admin bool
	// page is whether a person's browser asked, and so would rather be shown a page than a line of text for a thing that is not there.
	page bool
	// swallow is set once a page was written in place of the line of text a handler went on to write.
	swallow bool
}

// WriteHeader draws a page for a thing that is not there, in place of the line of plain text, when a person's browser asked. The window
// of the desktop app has no address bar, and a bare line leaves nothing in it to go back with.
func (m *marked) WriteHeader(code int) {
	if code == http.StatusNotFound && m.page && strings.HasPrefix(m.Header().Get("Content-Type"), "text/plain") {
		h := m.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		m.ResponseWriter.WriteHeader(code)
		_, _ = m.ResponseWriter.Write([]byte(strings.ReplaceAll(notFoundPage, "{{nonce}}", m.nonce)))
		m.swallow = true
		return
	}
	m.ResponseWriter.WriteHeader(code)
}

func (m *marked) Write(body []byte) (int, error) {
	if m.swallow {
		return len(body), nil
	}
	return m.ResponseWriter.Write(body)
}

const notFoundPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Not found</title><link rel="stylesheet" href="/static/tokens.css">
<style>
body { margin: 0; min-height: 100dvh; display: grid; place-items: center; background: var(--yg-bg); color: var(--yg-text); font: 15px/1.55 var(--yg-font-text); }
main { width: min(26rem, 92vw); display: grid; gap: var(--yg-space-4); padding: var(--yg-space-8); text-align: center; }
h1 { margin: 0; font: 600 1.375rem/1.2 var(--yg-font-display); letter-spacing: -.02em; }
p { margin: 0; color: var(--yg-text-muted); }
.row { display: flex; gap: var(--yg-space-3); justify-content: center; }
a { padding: var(--yg-space-2) var(--yg-space-4); border: 1px solid var(--yg-border-strong); border-radius: var(--yg-radius-sm); color: var(--yg-text); text-decoration: none; font-weight: 600; font-size: .875rem; }
a.main { background: var(--yg-accent); border-color: var(--yg-accent); color: var(--yg-on-accent); }
</style></head><body><main>
<h1>That page is not here</h1>
<p>It may have been moved or removed, or the link was not to a page of this site.</p>
<div class="row"><a href="#" id="back">Go back</a><a class="main" href="/chat">Chat</a></div>
</main><script nonce="{{nonce}}">document.getElementById("back").addEventListener("click", function (event) { event.preventDefault(); history.back(); });</script></body></html>`

// Unwrap lets a handler reach the connection's own controls, such as the write
// deadline a stream sets.
func (m *marked) Unwrap() http.ResponseWriter { return m.ResponseWriter }

func (m *marked) Flush() {
	if flusher, ok := m.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// markOf is what the response w was marked with, or nothing.
func markOf(w http.ResponseWriter) marked {
	for w != nil {
		switch held := w.(type) {
		case *marked:
			return marked{nonce: held.nonce, admin: held.admin}
		case interface{ Unwrap() http.ResponseWriter }:
			w = held.Unwrap()
		default:
			return marked{}
		}
	}
	return marked{}
}

// nonceOf is the nonce of the response w belongs to, or nothing.
func nonceOf(w http.ResponseWriter) string { return markOf(w).nonce }

const (
	// failuresAllowed is how many wrong credentials one source may present
	// within failureWindow before it is turned away for the rest of the window,
	// the right one included: a source that is guessing must not learn when it
	// has guessed.
	failuresAllowed = 10
	failureWindow   = 10 * time.Minute
)

// attempts counts wrong credentials by where they came from. Behind a proxy that
// this server does not believe, everybody has the proxy's address, and the limit
// is then shared; the identity provider is the way in for a team that size.
type attempts struct {
	mu      sync.Mutex
	sources map[string][]time.Time
}

// blocked says whether from has used up its tries.
func (a *attempts) blocked(from string, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.recent(from, now)) >= failuresAllowed
}

// failed counts a wrong credential.
func (a *attempts) failed(from string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sources == nil {
		a.sources = map[string][]time.Time{}
	}
	for source := range a.sources {
		if len(a.recent(source, now)) == 0 {
			delete(a.sources, source)
		}
	}
	a.sources[from] = append(a.recent(from, now), now)
}

func (a *attempts) recent(from string, now time.Time) []time.Time {
	kept := a.sources[from][:0:0]
	for _, at := range a.sources[from] {
		if now.Sub(at) < failureWindow {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(a.sources, from)
	} else {
		a.sources[from] = kept
	}
	return kept
}

// tooMany answers a source that has used up its tries.
func tooMany(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "600")
	http.Error(w, "too many wrong attempts from here: try again in ten minutes", http.StatusTooManyRequests)
}

// bodyLimit is the most a request may send. An integration upload is the one that sends more than a form.
func bodyLimit(r *http.Request) int64 {
	if r.URL.Path == integrationsPath+"/import" {
		return importLimit
	}
	return maxBody
}
