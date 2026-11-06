package main

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/settings"
)

// cloudSettings is every setting gusto cloud reads, read once when it starts.
type cloudSettings struct {
	records cloudRecords
	front   frontSettings
	signup  signupSettings
	demo    demoSettings
	billing billingSettings
	servers orgServerSettings
	cells   cellPoolSettings
	scaler  scalerSettings
}

func (c *cloudSettings) declare(set *settings.Set) {
	c.records.declare(set)
	c.front.declare(set)
	c.signup.declare(set)
	c.demo.declare(set)
	c.billing.declare(set)
	c.servers.declare(set)
	c.cells.declare(set)
	c.scaler.declare(set)
}

// loadCloud reads gusto cloud's settings from lookup, which is os.Getenv for the command and a map in a test.
func loadCloud(lookup func(name string) string) (cloudSettings, error) {
	var c cloudSettings
	set := settings.New()
	c.declare(set)
	return c, set.Load(lookup)
}

// cloudRecords is where gusto cloud keeps the accounts and the organizations' files, which every command of a gusto cloud's
// host reads.
type cloudRecords struct{ root, databaseURL string }

func (r *cloudRecords) declare(set *settings.Set) {
	set.String(&r.root, "GUSTO_CLOUD_ROOT", "cloud", "the directory gusto cloud keeps organizations' files in, and cloud.db when there is no database address")
	set.Secret(&r.databaseURL, "GUSTO_DATABASE_URL", "where the accounts are kept: empty for cloud.db under GUSTO_CLOUD_ROOT, or a postgres:// "+
		"address, whose search_path parameter names the schema, made when missing, and which several front doors can share")
}

func (r cloudRecords) accounts() cloud.Settings {
	return cloud.Settings{DatabaseURL: strings.TrimSpace(r.databaseURL)}
}

// frontSettings is where gusto cloud listens and is reached, how it sends mail, and what it asks of a visitor.
type frontSettings struct {
	addr, url, domain               string
	trustProxy                      bool
	mail, mailToken                 string
	leaseFile                       string
	leaseStepMinutes, leaderSeconds int
	leadHookURL, leadHookSecret     string
	turnstileSite, turnstileSecret  string
	perAddress, perMailbox, inAll   int
}

func (f *frontSettings) declare(set *settings.Set) {
	set.String(&f.addr, "GUSTO_CLOUD_ADDR", "127.0.0.1:8050", "where gusto cloud listens")
	set.String(&f.url, "GUSTO_CLOUD_URL", "", "the address links in mail begin with, such as https://cloud.example.com (default http:// and GUSTO_CLOUD_ADDR)")
	set.String(&f.domain, "GUSTO_CLOUD_DOMAIN", "localhost", "the domain organizations are served under, as <slug>.<domain>")
	set.Bool(&f.trustProxy, "GUSTO_CLOUD_TRUST_PROXY", "trust the address a proxy in front says a request came from")
	set.String(&f.mail, "GUSTO_CLOUD_MAIL", "outbox", "how mail is sent: outbox writes it to files; smtp://user:pass@host:587?from=a@example.com "+
		"sends it by SMTP; cloudflare://<account id>?from=a@example.com sends it by Cloudflare")
	set.Secret(&f.mailToken, "GUSTO_CLOUD_MAIL_TOKEN", "the Cloudflare API token that may send mail, for cloudflare://")
	set.String(&f.leaseFile, "GUSTO_CLOUD_LEASE_FILE", "", "the file that says when a demo host removes itself; a person signed in at /lease extends it, "+
		"and without it there is no lease")
	set.Int(&f.leaseStepMinutes, "GUSTO_CLOUD_LEASE_STEP_MINUTES", 30, 1, "how much each extension of the lease adds, in minutes")
	set.Int(&f.leaderSeconds, "GUSTO_CLOUD_LEADER_SECONDS", 30, 1, "how long the lease that makes one front door the leader lasts unrenewed, in seconds")
	set.String(&f.leadHookURL, "GUSTO_CLOUD_LEAD_HOOK_URL", "", "the address each demo lead is sent to, by a signed request")
	set.Secret(&f.leadHookSecret, "GUSTO_CLOUD_LEAD_HOOK_SECRET", "the secret the lead hook's requests are signed with")
	set.String(&f.turnstileSite, "GUSTO_CLOUD_TURNSTILE_SITE", "", "the Cloudflare Turnstile site key of the bot check on the demo form, which fails closed")
	set.Secret(&f.turnstileSecret, "GUSTO_CLOUD_TURNSTILE_SECRET", "the Cloudflare Turnstile secret of the bot check")
	set.Int(&f.perAddress, "GUSTO_CLOUD_PER_ADDRESS", 10, 1, "the most sign-ups, sign-ins and demo requests from one network address in an hour")
	set.Int(&f.perMailbox, "GUSTO_CLOUD_PER_MAILBOX", 3, 1, "the most mail sent to one address in an hour")
	set.Int(&f.inAll, "GUSTO_CLOUD_IN_ALL", 200, 1, "the most sign-ups, and the most demo requests, in all in an hour")
}

// public is the address links in mail begin with, which is also what the front door's cookies are scoped by.
func (f frontSettings) public() (*url.URL, error) {
	address := f.url
	if address == "" {
		address = "http://" + f.addr
	}
	public, err := url.Parse(address)
	if err != nil || (public.Scheme != "http" && public.Scheme != "https") || public.Host == "" {
		return nil, errors.New("GUSTO_CLOUD_URL is the address links in mail begin with, as https://cloud.example.com")
	}
	return public, nil
}

func (f frontSettings) lease() *cloud.Lease {
	if f.leaseFile == "" {
		return nil
	}
	return cloud.OpenLease(f.leaseFile, time.Duration(f.leaseStepMinutes)*time.Minute)
}

// signupSettings is who may sign up and what an organization may run: on the trial, once its address is confirmed, and
// once it pays.
type signupSettings struct {
	freeMail                             string
	refusedDomains                       []string
	perDomain                            int
	trialJobs, trialConcurrent           int
	jobs, concurrent, members, computers int
	paidJobsPerSeat, paidMembers         int
}

func (s *signupSettings) declare(set *settings.Set) {
	d := cloud.DefaultLimits()
	set.Choice(&s.freeMail, "GUSTO_CLOUD_FREE_MAIL", "refuse", []string{"allow", "refuse"}, "whether free mail providers may sign up; allow is for a demonstration")
	set.List(&s.refusedDomains, "GUSTO_CLOUD_REFUSED_DOMAINS", "more mail domains to turn away, beside the built-in ones, comma-separated")
	set.Int(&s.perDomain, "GUSTO_CLOUD_PER_DOMAIN", 3, 1, "how many organizations one mail domain may make")
	set.Int(&s.trialJobs, "GUSTO_CLOUD_TRIAL_JOBS", d.Trial.Jobs, 1, "the tasks in all an organization may run before its admin's address is confirmed")
	set.Int(&s.trialConcurrent, "GUSTO_CLOUD_TRIAL_CONCURRENT", d.Trial.Concurrent, 1, "the tasks at once an organization may run before its admin's address is confirmed")
	set.Int(&s.jobs, "GUSTO_CLOUD_JOBS", d.Verified.Jobs, 1, "the tasks a month a confirmed organization may run")
	set.Int(&s.concurrent, "GUSTO_CLOUD_CONCURRENT", d.Verified.Concurrent, 1, "the tasks at once a confirmed organization may run")
	set.Int(&s.members, "GUSTO_CLOUD_MEMBERS", d.Verified.Members, 1, "the people a confirmed organization may have")
	set.Int(&s.computers, "GUSTO_CLOUD_COMPUTERS", d.Verified.Computers, 1, "the workspaces a confirmed organization may have")
	set.Int(&s.paidJobsPerSeat, "GUSTO_CLOUD_PAID_JOBS_PER_SEAT", d.Paid.Jobs, 1, "the tasks a month a paying organization may run for each seat")
	set.Int(&s.paidMembers, "GUSTO_CLOUD_PAID_MEMBERS", d.Paid.Members, 1, "the people, and the workspaces, a paying organization may have")
}

func (s signupSettings) limits() cloud.Limits {
	limits := cloud.DefaultLimits()
	limits.Trial.Jobs, limits.Trial.Concurrent = s.trialJobs, s.trialConcurrent
	limits.Verified.Jobs, limits.Verified.Concurrent = s.jobs, s.concurrent
	limits.Verified.Members, limits.Verified.Computers = s.members, s.computers
	limits.Paid.Jobs, limits.Paid.Members, limits.Paid.Computers = s.paidJobsPerSeat, s.paidMembers, s.paidMembers
	return limits
}

// demoSettings is the demo gusto cloud offers at /demo, which it offers only when GUSTO_CLOUD_DEMO_MINUTES is set.
type demoSettings struct {
	minutes, extendMinutes, patienceMinutes int
	max, perDay, perPerson, perCompany      int
	bookURL, video, notify                  string
	staff                                   []string
}

func (d *demoSettings) declare(set *settings.Set) {
	set.Int(&d.minutes, "GUSTO_CLOUD_DEMO_MINUTES", 0, 0, "the minutes a demo lasts from when it is ready (default 0, no demo)")
	set.Int(&d.extendMinutes, "GUSTO_CLOUD_DEMO_EXTEND_MINUTES", 10, 1, "the minutes a demo's one extension adds")
	set.Int(&d.patienceMinutes, "GUSTO_CLOUD_DEMO_PATIENCE_MINUTES", 15, 1, "the minutes a demo has to be ready before it is deleted")
	set.Int(&d.max, "GUSTO_CLOUD_DEMO_MAX", 5, 1, "the most demos at once, until the operator's page sets it")
	set.Int(&d.perDay, "GUSTO_CLOUD_DEMO_PER_DAY", 40, 1, "the most demos started in a day, until the operator's page sets it")
	set.Int(&d.perPerson, "GUSTO_CLOUD_DEMO_PER_PERSON", 2, 1, "the most demos one address may start in a day, until the operator's page sets it")
	set.Int(&d.perCompany, "GUSTO_CLOUD_DEMO_PER_COMPANY", 6, 1, "the most demos one company may start in a day, until the operator's page sets it")
	set.String(&d.bookURL, "GUSTO_CLOUD_BOOK_URL", "", "where a visitor books a call")
	set.String(&d.video, "GUSTO_CLOUD_DEMO_VIDEO", "", "the video the landing page shows: an https address of a file or of a page to embed")
	set.String(&d.notify, "GUSTO_CLOUD_DEMO_NOTIFY", "", "an address mailed when a visitor starts a demo")
	set.List(&d.staff, "GUSTO_CLOUD_DEMO_STAFF", "the addresses and domains whose people alone may start a demo, comma-separated (default anyone)")
}

func (d demoSettings) forCloud() cloud.DemoSettings {
	if d.minutes == 0 {
		return cloud.DemoSettings{}
	}
	return cloud.DemoSettings{
		Length:     time.Duration(d.minutes) * time.Minute,
		Extend:     time.Duration(d.extendMinutes) * time.Minute,
		Patience:   time.Duration(d.patienceMinutes) * time.Minute,
		MaxOrgs:    d.max,
		PerDay:     d.perDay,
		PerPerson:  d.perPerson,
		PerCompany: d.perCompany,
		BookURL:    d.bookURL,
		VideoURL:   d.video,
		Staff:      lowered(d.staff),
	}
}

// billingSettings is how gusto cloud bills seats through Stripe.
type billingSettings struct {
	key, price, webhookSecret string
	seatUSD                   int
}

func (b *billingSettings) declare(set *settings.Set) {
	set.Secret(&b.key, "GUSTO_CLOUD_STRIPE_KEY", "the Stripe secret API key organizations are billed by the seat with; the key, the price and the webhook secret are set together or not at all")
	set.String(&b.price, "GUSTO_CLOUD_STRIPE_PRICE", "", "the Stripe price of one seat for a month")
	set.Secret(&b.webhookSecret, "GUSTO_CLOUD_STRIPE_WEBHOOK_SECRET", "the signing secret of the Stripe webhook")
	set.Int(&b.seatUSD, "GUSTO_CLOUD_SEAT_USD", 25, 1, "the seat's price the organization page shows, in dollars")
}

// stripe is how gusto cloud bills seats, and nil when it bills nothing. Setting some of the three and not all is refused, so
// a front door never takes payments it cannot hear about.
func (b billingSettings) stripe() (*cloud.Stripe, error) {
	s := &cloud.Stripe{Key: b.key, Price: b.price, WebhookSecret: b.webhookSecret, SeatUSD: b.seatUSD}
	switch {
	case s.Enabled():
		return s, nil
	case s.Key == "" && s.Price == "" && s.WebhookSecret == "":
		return nil, nil
	}
	return nil, errors.New("billing needs GUSTO_CLOUD_STRIPE_KEY, GUSTO_CLOUD_STRIPE_PRICE and GUSTO_CLOUD_STRIPE_WEBHOOK_SECRET together")
}

// orgServerSettings is how gusto cloud starts each organization's gusto serve.
type orgServerSettings struct {
	run                        string
	orgEnv, demoEnv, indexDir  string
	portFrom, startWaitSeconds int
}

func (o *orgServerSettings) declare(set *settings.Set) {
	set.Choice(&o.run, "GUSTO_CLOUD_SERVERS", "on", []string{"on", "off"}, "whether gusto cloud starts organizations' servers; off keeps the accounts only")
	set.String(&o.orgEnv, "GUSTO_CLOUD_ORG_ENV", "", "a file of KEY=VALUE lines every organization's server starts with: the operator's credentials, models "+
		"and sandbox; its GUSTO_DATABASE_URL, when it is a postgres:// address, gives each organization a schema of its own for its indexes")
	set.String(&o.demoEnv, "GUSTO_CLOUD_DEMO_ENV", "", "a file of KEY=VALUE lines a demo organization's server starts with on top of GUSTO_CLOUD_ORG_ENV's, such as another sandbox")
	set.String(&o.indexDir, "GUSTO_CLOUD_INDEX_DIR", "", "where organizations' indexes are kept when that address is not Postgres: a directory for each, "+
		"on the local disk of the host its server runs on (default index under GUSTO_CLOUD_ROOT)")
	set.Int(&o.portFrom, "GUSTO_CLOUD_PORT_FROM", 9100, 1, "the first port an organization's server gets, two each")
	set.Int(&o.startWaitSeconds, "GUSTO_CLOUD_START_WAIT_SECONDS", 120, 1, "how long a server has to answer once started, in seconds")
}
