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
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		next.ServeHTTP(&nonced{ResponseWriter: w, nonce: nonce}, r)
	})
}

// nonced carries the request's nonce to the page that draws it.
type nonced struct {
	http.ResponseWriter
	nonce string
}

// Unwrap lets a handler reach the connection's own controls, such as the write
// deadline a stream sets.
func (n *nonced) Unwrap() http.ResponseWriter { return n.ResponseWriter }

func (n *nonced) Flush() {
	if flusher, ok := n.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// nonceOf is the nonce of the response w belongs to, or nothing.
func nonceOf(w http.ResponseWriter) string {
	for w != nil {
		switch held := w.(type) {
		case *nonced:
			return held.nonce
		case interface{ Unwrap() http.ResponseWriter }:
			w = held.Unwrap()
		default:
			return ""
		}
	}
	return ""
}

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
