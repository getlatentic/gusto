package cloud

import (
	"embed"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/web"
)

//go:embed templates/*.html static/cloud.css static/hero-light.webp static/hero-dark.webp static/shots/*.webp static/landing-theme.js static/og.png
var files embed.FS

// Config is what a front door is made from.
type Config struct {
	Store    *Store
	Mail     Mailer
	Sessions *Sessions
	// URL is the address links in mail begin with, and Domain is the domain
	// organizations are served under, as <slug>.<domain>.
	URL    *url.URL
	Domain string
	// TrustProxy says the address of a request is the last one a proxy in front
	// of this server wrote in X-Forwarded-For, and not the connection's own.
	TrustProxy bool
	// The most sign-ups from one address in an hour, mail to one address in an
	// hour, and sign-ups in all in an hour.
	PerAddress, PerMailbox, InAll int
	// Controller runs the organizations' servers. Without one the front door has
	// accounts and no organization to open.
	Controller *Controller
	// Lease, when there is one, is how long this host has before it removes itself.
	Lease *Lease
	// Turnstile is the bot check on the demo form, when it is set.
	Turnstile Turnstile
	// Leads sends each lead to where it is followed up, when it is set.
	Leads *LeadHook
	// Notify is the address mailed when a visitor starts a demo, when it is set.
	Notify string
	// ComputersWait is how long an ending demo waits for its computers to be removed. It is ninety seconds when not set.
	ComputersWait time.Duration
}

// Server is the front door's pages.
type Server struct {
	Config
	pages   *template.Template
	signups *Limiter
	mails   *Limiter
	all     *Limiter
	invites *Limiter
	sending sync.WaitGroup
	making  sync.WaitGroup
	ender   demoEnder
}

// New makes the front door.
func New(c Config) (*Server, error) {
	if c.PerAddress <= 0 {
		c.PerAddress = 10
	}
	if c.PerMailbox <= 0 {
		c.PerMailbox = 3
	}
	if c.InAll <= 0 {
		c.InAll = 200
	}
	pages, err := template.New("").Funcs(template.FuncMap{"asset": web.Asset, "own": fingerprinted, "minutes": minutes, "minuteLong": minuteLong, "clock": clock, "picture": picture, "darkScreen": darkScreen}).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	if err := c.Turnstile.Complete(); err != nil {
		return nil, err
	}
	if err := c.Leads.Complete(); err != nil {
		return nil, err
	}
	return &Server{Config: c, pages: pages,
		signups: c.limiter("signups", c.PerAddress), mails: c.limiter("mails", c.PerMailbox), all: c.limiter("all", c.InAll),
		invites: c.limiter("invites", 20)}, nil
}

// limiter counts most events an hour. With a store the count is shared by every front door on its database.
func (c Config) limiter(scope string, most int) *Limiter {
	if c.Store == nil {
		return NewLimiter(most, time.Hour)
	}
	return c.Store.Limiter(scope, most, time.Hour)
}

// Handler is every page the front door has, and every organization's server behind
// its own host name.
func (s *Server) Handler() http.Handler {
	pages := s.routes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Asked on every host name, so a page of an organization can warn of the end of the host.
		if r.Method == http.MethodGet && r.URL.Path == "/.gusto/lease" {
			if slug, ok := s.orgSlug(r.Host); ok && s.demoLeaseJSON(w, slug) {
				return
			}
			s.leaseJSON(w, r)
			return
		}
		if _, _, ok := s.appHost(r.Host); ok && unlisted(w, r) {
			return
		}
		if slug, id, ok := s.appHost(r.Host); ok {
			s.routeApp(w, r, slug, id)
			return
		}
		if _, ok := s.orgSlug(r.Host); ok && unlisted(w, r) {
			return
		}
		if slug, ok := s.orgSlug(r.Host); ok {
			// The page of a demo that has ended is made of the front door's own styles, which the gone server cannot send.
			if _, found := s.Store.LeadOf(slug); found && strings.HasPrefix(r.URL.Path, "/static/") && !s.orgExists(slug) {
				pages.ServeHTTP(w, r)
				return
			}
			s.route(w, r, slug)
			return
		}
		pages.ServeHTTP(w, r)
	})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /signup", s.signUpPage)
	mux.HandleFunc("POST /signup", s.signUp)
	mux.HandleFunc("GET /signin", s.signInPage)
	mux.HandleFunc("POST /signin", s.signIn)
	mux.HandleFunc("GET /verify", s.confirmPage(Verify, "Confirm your email address", "Confirm"))
	mux.HandleFunc("POST /verify", s.useLink(Verify))
	mux.HandleFunc("GET /signin/link", s.confirmPage(SignIn, "Sign in to gusto", "Continue"))
	mux.HandleFunc("POST /signin/link", s.useLink(SignIn))
	mux.HandleFunc("GET /org", s.guard(s.org))
	mux.HandleFunc("POST /org/verify", s.guard(s.resend))
	mux.HandleFunc("GET /open", s.guard(s.open))
	mux.HandleFunc("POST /org/invite", s.guard(s.invite))
	mux.HandleFunc("POST /org/invites/cancel", s.guard(s.cancelInvitation))
	mux.HandleFunc("POST /org/people/remove", s.guard(s.removePerson))
	mux.HandleFunc("POST /org/people/role", s.guard(s.changeRole))
	mux.HandleFunc("GET /invite", s.invitationPage)
	mux.HandleFunc("POST /invite", s.acceptInvitation)
	mux.HandleFunc("GET /demo", s.demoForm)
	mux.HandleFunc("POST /demo", s.startDemo)
	mux.HandleFunc("GET /demo/wait", s.guard(s.demoWait))
	mux.HandleFunc("POST /demo/extend", s.guard(s.extendDemo))
	mux.HandleFunc("GET /demo/end", s.guard(s.stopDemoPage))
	mux.HandleFunc("POST /demo/end", s.guard(s.stopDemo))
	mux.HandleFunc("GET /demo/ended/{slug}", s.demoEndedPage)
	mux.HandleFunc("GET /lease", s.guard(s.leasePage))
	mux.HandleFunc("POST /lease/extend", s.guard(s.extendLease))
	mux.HandleFunc("POST /signout", s.signOut)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
	mux.Handle("GET /static/cloud.css", cached(http.FileServerFS(files)))
	mux.Handle("GET /static/shots/", cached(http.FileServerFS(files)))
	mux.Handle("GET /static/hero-light.webp", cached(http.FileServerFS(files)))
	mux.Handle("GET /static/hero-dark.webp", cached(http.FileServerFS(files)))
	mux.Handle("GET /static/landing-theme.js", cached(http.FileServerFS(files)))
	mux.Handle("GET /static/og.png", cached(http.FileServerFS(files)))
	mux.HandleFunc("GET /robots.txt", s.robots)
	mux.HandleFunc("GET /sitemap.xml", s.sitemap)
	mux.Handle("GET /static/", web.Static())
	return s.secure(mux)
}

// Wait returns when every mail started so far has been handed on, and every
// organization started so far has answered or failed.
func (s *Server) Wait() {
	s.sending.Wait()
	s.making.Wait()
}

// secure gives every response headers that keep a page from being framed,
// sniffed or scripted, refuses a form that another site sent, and keeps a
// request from sending more than a form is.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if s.Sessions.secure {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			h.Set("Cache-Control", "no-store")
		}
		if r.Method == http.MethodPost {
			if !sameSite(r) {
				http.Error(w, "that form was sent from another site", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		}
		next.ServeHTTP(w, r)
	})
}

// sameSite says a form came from a page of this server. A browser names the
// page's origin on every form it posts, and says when the site is another.
func sameSite(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		return err == nil && parsed.Host == r.Host
	}
	return r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

func cached(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		next.ServeHTTP(w, r)
	})
}

// source is the address a request came from, for the limits on it.
func (s *Server) source(r *http.Request) string {
	if s.TrustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// send mails a person without making the request wait for it, so how long it
// takes says nothing about whether the address has an account.
func (s *Server) send(to, subject, body string) {
	s.sending.Add(1)
	go func() {
		defer s.sending.Done()
		if err := s.Mail.Send(to, subject, body); err != nil {
			log.Printf("cloud: mail to a person was not sent: %v", err)
		}
	}()
}
