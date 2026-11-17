package cloud

import (
	"embed"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/deploy/install"
	"github.com/getlatentic/gusto/internal/web"
)

//go:embed static/og.png
var files embed.FS

// Config is what a gusto cloud server is made from.
type Config struct {
	Store    *Store
	Mail     Mailer
	Sessions *Sessions
	// URL is the address links in mail begin with, and Domain is the domain
	// organizations are served under, as <slug>.<domain>, when they are not at paths.
	URL    *url.URL
	Domain string
	// OrgPaths says organizations are reached at /workspace/<slug> on URL's own name, and not at host names of their own.
	OrgPaths bool
	// TrustProxy says the address of a request is the last one a proxy in front
	// of this server wrote in X-Forwarded-For, and not the connection's own.
	TrustProxy bool
	// The most sign-ups from one address in an hour, mail to one address in an
	// hour, and sign-ups in all in an hour.
	PerAddress, PerMailbox, InAll int
	// Controller runs the organizations' servers. Without one gusto cloud has
	// accounts and no organization to open.
	Controller *Controller
	// Lease, when there is one, is how long this host has before it removes itself.
	Lease *Lease
	// Google is signing in with a Google account, when gusto cloud has a client at Google.
	Google *Google
	// Turnstile is the bot check on the demo form, when it is set.
	Turnstile Turnstile
	// Leads sends each lead to where it is followed up, when it is set.
	Leads *LeadHook
	// Notify is the address mailed when a visitor starts a demo, when it is set.
	Notify string
	// ComputersWait is how long an ending demo waits for its computers to be removed. It is ninety seconds when not set.
	ComputersWait time.Duration
	// Prewarm, when it is set, starts a node for a visitor who has begun the demo form, so it is up by the time they send it.
	// It must not block, and must bound what it starts itself.
	Prewarm func()
	// Stripe bills organizations for their seats, when it is set up, and Seats keeps the seats billed equal to the members.
	Stripe *Stripe
	Seats  *SeatSync
	// Metrics is what this process reports. New makes one when it is not given, and gives it to the controller.
	Metrics *Metrics
}

// Server is gusto cloud's pages.
type Server struct {
	Config
	signups *Limiter
	mails   *Limiter
	all     *Limiter
	invites *Limiter
	codes   *Limiter
	sending sync.WaitGroup
	making  sync.WaitGroup
	ender   demoEnder
}

// New makes gusto cloud.
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
	if err := c.Turnstile.Complete(); err != nil {
		return nil, err
	}
	if err := c.Leads.Complete(); err != nil {
		return nil, err
	}
	if c.Metrics == nil {
		c.Metrics = NewMetrics()
	}
	if c.Controller != nil && c.Controller.Metrics == nil {
		c.Controller.Metrics = c.Metrics
		c.Metrics.watch(c.Controller)
	}
	return &Server{Config: c,
		signups: c.limiter("signups", c.PerAddress), mails: c.limiter("mails", c.PerMailbox), all: c.limiter("all", c.InAll),
		invites: c.limiter("invites", 20), codes: c.limiter("codes", codeTries)}, nil
}

// limiter counts most events an hour. With a store the count is shared by every gusto cloud server on its database.
func (c Config) limiter(scope string, most int) *Limiter {
	if c.Store == nil {
		return NewLimiter(most, time.Hour)
	}
	return c.Store.Limiter(scope, most, time.Hour)
}

// Handler is every page gusto cloud has, and every organization's server behind
// its own host name.
func (s *Server) Handler() http.Handler {
	served := s.Metrics.observe(s.serve(), s.routeClass)
	metrics := onlyHere(s.Metrics.Handler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == MetricsPath && r.Method == http.MethodGet {
			metrics.ServeHTTP(w, r)
			return
		}
		served.ServeHTTP(w, r)
	})
}

// routeClass names the kind of a request that was served, for the metrics: the mux pattern that matched, or the organization's
// server or computer's app it was sent on to. It is read after the request, which is when the mux has set the pattern.
func (s *Server) routeClass(r *http.Request) string {
	switch {
	case r.URL.Path == "/.gusto/lease":
		return "lease"
	case r.Pattern != "":
		return r.Pattern
	}
	if _, _, ok := s.appHost(r.Host); ok {
		return "app"
	}
	if _, ok := s.orgSlug(r.Host); ok {
		return "org"
	}
	if _, _, ok := s.orgAtPath(r.URL.Path); ok {
		return "org"
	}
	return "unmatched"
}

func (s *Server) serve() http.Handler {
	pages := s.routes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slug, rest, ok := s.orgAtPath(r.URL.Path); ok {
			s.serveOrg(w, mountedRequest(r, slug, rest), pages, slug, orgPath+slug)
			return
		}
		slug, atHost := s.orgSlug(r.Host)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/.gusto/lease":
			s.lease(w, r, slug)
		case s.isAppHost(r.Host):
			s.serveApp(w, r)
		case atHost && s.OrgPaths:
			// An organization's host name, from before organizations were at paths, is where a kept link still finds it.
			http.Redirect(w, r, s.orgURL(slug)+r.URL.RequestURI(), http.StatusPermanentRedirect)
		case atHost:
			s.serveOrg(w, r, pages, slug, "")
		default:
			pages.ServeHTTP(w, r)
		}
	})
}

func (s *Server) isAppHost(host string) bool { _, _, ok := s.appHost(host); return ok }

// lease is asked at every organization's address, so a page of an organization can warn of the end of the host. slug is the
// organization asked at, and "" at gusto cloud's own pages.
func (s *Server) lease(w http.ResponseWriter, r *http.Request, slug string) {
	if slug != "" && s.demoLeaseJSON(r.Context(), w, slug) {
		return
	}
	s.leaseJSON(w, r)
}

func (s *Server) serveApp(w http.ResponseWriter, r *http.Request) {
	if unlisted(w, r) {
		return
	}
	slug, id, _ := s.appHost(r.Host)
	s.routeApp(w, r, slug, id)
}

// serveOrg answers a request for an organization's pages. prefix is the path the organization is at on gusto cloud's own
// name, and "" when it has a host name of its own.
func (s *Server) serveOrg(w http.ResponseWriter, r *http.Request, pages http.Handler, slug, prefix string) {
	if unlisted(w, r) {
		return
	}
	if prefix != "" && r.Method == http.MethodGet && r.URL.Path == "/.gusto/lease" {
		s.lease(w, r, slug)
		return
	}
	// The page of a demo that has ended is drawn by gusto cloud's own files and the app's, which the gone server cannot send.
	if _, found := s.Store.LeadOf(r.Context(), slug); found && servedByCloud(r.URL.Path) && !s.orgExists(r.Context(), slug) {
		pages.ServeHTTP(w, r)
		return
	}
	s.route(w, r, slug, prefix)
}

func servedByCloud(path string) bool {
	return strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/app/assets/")
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /app", s.app)
	mux.HandleFunc("GET /signup", s.signUpPage)
	mux.HandleFunc("POST /signup", s.signUp)
	mux.HandleFunc("GET /signin", s.signInPage)
	mux.HandleFunc("POST /signin", s.signIn)
	mux.HandleFunc("GET /verify", s.confirmPage(Verify, "Confirm your email address", "Confirm"))
	mux.HandleFunc("POST /verify", s.useLink(Verify))
	mux.HandleFunc("GET /signin/link", s.confirmPage(SignIn, "Sign in to gusto", "Continue"))
	mux.HandleFunc("POST /signin/link", s.useLink(SignIn))
	mux.HandleFunc("POST /signin/code", s.signInWithCode)
	s.googleRoutes(mux)
	mux.HandleFunc("GET /orgs", s.orgsPage)
	mux.HandleFunc("GET /orgs.json", s.orgsJSON)
	mux.HandleFunc("GET /account.json", s.accountJSON)
	mux.HandleFunc("POST /account/confirm", s.confirmAgain)
	mux.HandleFunc("GET /org", s.guard(s.org))
	mux.HandleFunc("POST /org/verify", s.guard(s.resend))
	mux.HandleFunc("GET /open", s.guard(s.open))
	s.peopleRoutes(mux)
	mux.HandleFunc("POST /org/billing/subscribe", s.guard(s.subscribe))
	mux.HandleFunc("POST /org/billing/manage", s.guard(s.manageBilling))
	mux.HandleFunc("POST /stripe/webhook", s.stripeWebhook)
	mux.HandleFunc("GET /invite", s.invitationPage)
	mux.HandleFunc("POST /invite", s.acceptInvitation)
	mux.HandleFunc("GET /demo", s.demoForm)
	mux.HandleFunc("POST /demo", s.startDemo)
	mux.HandleFunc("POST /demo/warm", s.warmDemo)
	mux.HandleFunc("GET /demo/wait", s.guard(s.demoWait))
	mux.HandleFunc("POST /demo/extend", s.guard(s.extendDemo))
	mux.HandleFunc("GET /demo/colleague", s.guard(s.colleaguePage))
	mux.HandleFunc("POST /demo/colleague", s.guard(s.inviteColleague))
	mux.HandleFunc("GET /demo/end", s.guard(s.stopDemoPage))
	mux.HandleFunc("POST /demo/end", s.guard(s.stopDemo))
	mux.HandleFunc("GET /demo/ended/{slug}", s.demoEndedPage)
	mux.HandleFunc("GET /lease", s.guard(s.leasePage))
	mux.HandleFunc("POST /lease/extend", s.guard(s.extendLease))
	mux.HandleFunc("POST /signout", s.signOut)
	mux.HandleFunc("GET /signout", s.signedOutThere)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
	s.fileRoutes(mux)
	mux.HandleFunc("GET /robots.txt", s.robots)
	mux.HandleFunc("GET /sitemap.xml", s.sitemap)
	mux.Handle("GET /static/", web.Static())
	return s.secure(mux)
}

// fileRoutes are the files the front door serves as they are: its pictures, the app's code, and the script that installs gusto.
func (s *Server) fileRoutes(mux *http.ServeMux) {
	mux.Handle("GET /static/og.png", cached(http.FileServerFS(files)))
	mux.Handle("GET /app/assets/", web.AppAssets())
	mux.Handle("GET /install.sh", installer(install.Script, "text/x-shellscript"))
	mux.Handle("GET /install.ps1", installer(install.PowerShell, "text/plain"))
}

// installer answers with an install script, so `curl -fsSL <front door>/install.sh | sh` installs the newest release: GitHub's own
// latest-release address skips pre-releases.
func installer(script []byte, kind string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", kind+"; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(script)
	})
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
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; form-action 'self'"+s.orgsSource()+" https://checkout.stripe.com https://billing.stripe.com; frame-ancestors 'none'; base-uri 'none'; object-src 'none'")
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
			// An organization's pages ask for the confirmation link again, and change a person's role, from their own address.
			if !sameSite(r) && !(sentFromOrgs[r.URL.Path] && s.isOrgOrigin(r.Header.Get("Origin"))) {
				http.Error(w, "that form was sent from another site", http.StatusForbidden)
				return
			}
			// An event from Stripe is bigger than a form, and its handler bounds it itself.
			if r.URL.Path != "/stripe/webhook" {
				r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// sentFromOrgs are the forms an organization's own pages send here.
var sentFromOrgs = map[string]bool{"/account/confirm": true, "/org/people/role": true, "/org/invite": true}

// sameSite says a form came from a page of this server. A browser names the
// page's origin on every form it posts, and says when the site is another.
// orgsSource is every organization's and app's address, for the content security policy: signing in, signing up and
// joining are forms whose redirects end in one, and a browser checks each redirect of a form against form-action.
func (s *Server) orgsSource() string {
	if s.Domain == "" {
		return ""
	}
	source := " " + s.URL.Scheme + "://*." + s.Domain
	if port := s.URL.Port(); port != "" {
		source += ":" + port
	}
	return source
}

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
			slog.Error("mail to a person was not sent", "error", err)
		}
	}()
}
