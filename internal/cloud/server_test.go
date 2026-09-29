package cloud

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// front is a front door on a real listener, with a browser's cookies and mail
// that lands in a directory.
type front struct {
	t      *testing.T
	server *Server
	http   *httptest.Server
	store  *Store
	outbox string
}

func newFront(t *testing.T, set Settings, tune func(*Config)) *front {
	t.Helper()
	root := filepath.Join(t.TempDir(), "cloud")
	store, err := Open(root, set)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := OpenSessions(root, false)
	if err != nil {
		t.Fatal(err)
	}
	outbox := filepath.Join(root, "outbox")
	base, _ := url.Parse("http://cloud.test")
	config := Config{Store: store, Mail: Outbox{Dir: outbox, From: "haas <no-reply@cloud.test>"}, Sessions: sessions, URL: base, Domain: "cloud.test"}
	if tune != nil {
		tune(&config)
	}
	server, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	listener := httptest.NewServer(server.Handler())
	t.Cleanup(listener.Close)
	return &front{t: t, server: server, http: listener, store: store, outbox: outbox}
}

// browser is one person's cookies.
type browser struct {
	f      *front
	client *http.Client
}

func (f *front) browser() *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{f: f, client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (b *browser) do(method, path string, form url.Values, header map[string]string) (int, string, http.Header) {
	b.f.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, b.f.http.URL+path, body)
	if err != nil {
		b.f.t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if method == http.MethodPost {
		req.Header.Set("Origin", b.f.http.URL)
	}
	for k, v := range header {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := b.client.Do(req)
	if err != nil {
		b.f.t.Fatal(err)
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(text), resp.Header
}

func (b *browser) get(path string) (int, string) { c, t, _ := b.do("GET", path, nil, nil); return c, t }
func (b *browser) post(path string, form url.Values) (int, string, http.Header) {
	return b.do("POST", path, form, nil)
}

func signup(email, org string) url.Values { return url.Values{"email": {email}, "org": {org}} }

// mails is every message in the outbox, oldest first, after the ones being sent are.
func (f *front) mails() []string {
	f.server.Wait()
	entries, _ := os.ReadDir(f.outbox)
	var out []string
	for _, entry := range entries {
		text, _ := os.ReadFile(filepath.Join(f.outbox, entry.Name()))
		out = append(out, string(text))
	}
	return out
}

var linkInMail = regexp.MustCompile(`http://cloud\.test(/[^\s]+)`)

func (f *front) lastLink() string {
	f.t.Helper()
	mails := f.mails()
	if len(mails) == 0 {
		f.t.Fatal("no mail was sent")
	}
	found := linkInMail.FindStringSubmatch(mails[len(mails)-1])
	if found == nil {
		f.t.Fatalf("the mail holds no link:\n%s", mails[len(mails)-1])
	}
	return found[1]
}

func TestSigningUpMakesTheOrganisationSignsThePersonInAndMailsALink(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	b := f.browser()
	status, _, header := b.post("/signup", signup("Ada@Acme.com", "Acme Research"))
	if status != http.StatusSeeOther || header.Get("Location") != "/org" {
		t.Fatalf("%d %v", status, header)
	}
	status, page := b.get("/org")
	if status != 200 {
		t.Fatalf("the person should be signed in: %d", status)
	}
	for _, want := range []string{"Acme Research", "acme-com.cloud.test", "acme.com", "ada@acme.com", "Not verified", "3 in all, 1 at a time", "200 a month", "0 of 1"} {
		if !strings.Contains(page, want) {
			t.Errorf("the organisation page lacks %q:\n%s", want, page)
		}
	}
	if mails := f.mails(); len(mails) != 1 || !strings.Contains(mails[0], "To: ada@acme.com") || !strings.Contains(mails[0], "/verify?t=") {
		t.Errorf("one verification mail should have been sent: %v", mails)
	}
	quota, err := f.store.Quota("acme-com")
	if err != nil || quota.Jobs != 3 || quota.Concurrent != 1 {
		t.Errorf("an organisation that is not verified runs a small trial: %+v %v", quota, err)
	}
}

func TestFollowingTheLinkVerifiesAndLiftsTheLimitOnlyWhenItIsPressed(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	b := f.browser()
	b.post("/signup", signup("ada@acme.com", "Acme"))
	link := f.lastLink()

	// A mail program that opens the link to check it must not spend it.
	other := f.browser()
	for range 2 {
		if status, page := other.get(link); status != 200 || !strings.Contains(page, `action="/verify"`) || !strings.Contains(page, `name="t"`) {
			t.Fatalf("opening the link should ask for a press: %d\n%s", status, page)
		}
	}
	if person, _, _ := f.store.Person("ada@acme.com"); !person.Trial() {
		t.Fatal("opening the link verified the address")
	}

	token := strings.TrimPrefix(link[strings.Index(link, "t="):], "t=")
	status, _, header := other.post("/verify", url.Values{"t": {token}})
	if status != http.StatusSeeOther || header.Get("Location") != "/org" {
		t.Fatalf("%d %v", status, header)
	}
	if person, _, _ := f.store.Person("ada@acme.com"); person.Trial() {
		t.Error("pressing the button should verify the address")
	}
	if quota, _ := f.store.Quota("acme-com"); quota != DefaultLimits().Verified {
		t.Errorf("verifying lifts the limit: %+v", quota)
	}
	if _, page := other.get("/org"); !strings.Contains(page, "Verified") || !strings.Contains(page, "200 a month, 4 at a time") || !strings.Contains(page, "0 of 4") || !strings.Contains(page, "up to 4") {
		t.Errorf("the page should say so, and the browser that followed the link is signed in:\n%s", page)
	}
	if status, _, _ := f.browser().post("/verify", url.Values{"t": {token}}); status != http.StatusGone {
		t.Errorf("the link works once: %d", status)
	}
}

func TestASignUpThatIsRefusedSaysWhyAndKeepsWhatWasTyped(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	f.browser().post("/signup", signup("ada@acme.com", "Acme"))
	for name, c := range map[string]struct {
		form   url.Values
		status int
		says   string
	}{
		"free mail":       {signup("bob@gmail.com", "Bob Ltd"), 422, "email address your company gave you"},
		"disposable":      {signup("bob@mailinator.com", "Bob Ltd"), 422, "expire"},
		"not an email":    {signup("bob", "Bob Ltd"), 422, "not an email address"},
		"a taken address": {url.Values{"email": {"bob@bob.com"}, "org": {"Bob Ltd"}, "slug": {"acme-com"}}, 409, "is taken"},
		"a taken email":   {signup("ada@acme.com", "Another"), 409, "already has an account"},
		"a bad name":      {signup("bob@bob.com", "x"), 422, "2 to 80 characters"},
	} {
		status, page, _ := f.browser().post("/signup", c.form)
		if status != c.status || !strings.Contains(page, c.says) {
			t.Errorf("%s: %d\n%s", name, status, page)
		}
		if !strings.Contains(page, `value="`+c.form.Get("org")+`"`) {
			t.Errorf("%s: the organisation's name should still be in the form", name)
		}
	}
	if len(f.mails()) != 1 {
		t.Error("a refused sign-up sent mail")
	}
}

func TestAFormFromAnotherSiteIsRefused(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	b := f.browser()
	for name, header := range map[string]map[string]string{
		"another origin":  {"Origin": "https://evil.example"},
		"a cross-site":    {"Origin": "", "Sec-Fetch-Site": "cross-site"},
		"an origin whose": {"Origin": "http://cloud.test.evil.example"},
	} {
		if status, _, _ := b.do("POST", "/signup", signup("ada@acme.com", "Acme"), header); status != http.StatusForbidden {
			t.Errorf("%s: %d", name, status)
		}
	}
	if status, _, _ := b.do("POST", "/signup", signup("ada@acme.com", "Acme"), map[string]string{"Origin": ""}); status != http.StatusSeeOther {
		t.Errorf("a client that names no site, like curl, is not a browser at risk: %d", status)
	}
}

func TestSignUpsFromOneAddressAreLimitedAndProxiesAreTrustedOnlyWhenTold(t *testing.T) {
	f := newFront(t, Settings{PerDomain: 100}, func(c *Config) { c.PerAddress = 2 })
	for i, want := range []int{303, 303, 429} {
		if status, _, _ := f.browser().post("/signup", signup("p"+string(rune('a'+i))+"@acme"+string(rune('a'+i))+".com", "Team "+string(rune('a'+i)))); status != want {
			t.Errorf("sign-up %d: %d, want %d", i, status, want)
		}
	}
	// Without trust, a header naming another address changes nothing.
	b := f.browser()
	status, _, _ := b.do("POST", "/signup", signup("pz@acmez.com", "Team z"), map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if status != 429 {
		t.Errorf("the header of a client is not to be believed: %d", status)
	}

	behind := newFront(t, Settings{PerDomain: 100}, func(c *Config) { c.PerAddress = 1; c.TrustProxy = true })
	for i, address := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.1"} {
		name := string(rune('a' + i))
		status, _, _ := behind.browser().do("POST", "/signup", signup("q"+name+"@corp"+name+".com", "Org "+name), map[string]string{"X-Forwarded-For": "198.51.100.7, " + address})
		if want := []int{303, 303, 429}[i]; status != want {
			t.Errorf("behind a proxy, sign-up %d from %s: %d, want %d", i, address, status, want)
		}
	}
}

func TestSignUpsInAllAreLimited(t *testing.T) {
	f := newFront(t, Settings{PerDomain: 100}, func(c *Config) { c.InAll = 2; c.PerAddress = 100 })
	for i, want := range []int{303, 303, 503} {
		name := string(rune('a' + i))
		if status, _, _ := f.browser().post("/signup", signup("u"+name+"@corp"+name+".com", "Org "+name)); status != want {
			t.Errorf("sign-up %d: %d, want %d", i, status, want)
		}
	}
}

// The form cannot be used to learn who has an account: the answer is the same
// bytes for an address that has one and one that has not, and only the first is mailed.
func TestSigningInAnswersTheSameWhetherOrNotTheAddressHasAnAccount(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	f.browser().post("/signup", signup("ada@acme.com", "Acme"))
	before := len(f.mails())

	knownStatus, known, _ := f.browser().post("/signin", url.Values{"email": {"ada@acme.com"}})
	unknownStatus, unknown, _ := f.browser().post("/signin", url.Values{"email": {"nobody@acme.com"}})
	junkStatus, junk, _ := f.browser().post("/signin", url.Values{"email": {"not an address"}})
	if knownStatus != 200 || unknownStatus != knownStatus || junkStatus != knownStatus || known != unknown || known != junk {
		t.Errorf("the answers differ:\n%d %q\n%d %q\n%d %q", knownStatus, known, unknownStatus, unknown, junkStatus, junk)
	}
	mails := f.mails()
	if len(mails) != before+1 || !strings.Contains(mails[len(mails)-1], "To: ada@acme.com") || !strings.Contains(mails[len(mails)-1], "/signin/link?t=") {
		t.Errorf("only the address with an account is mailed: %d mails", len(mails)-before)
	}
}

func TestASignInLinkSignsThePersonInOnceAndTheSessionEndsWhenTheySignOut(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	f.browser().post("/signup", signup("ada@acme.com", "Acme"))
	f.mails() // mail is sent in the background, so the sign-up's is waited for before the next is asked for
	f.browser().post("/signin", url.Values{"email": {"ada@acme.com"}})
	link := f.lastLink()

	b := f.browser()
	if status, _ := b.get("/org"); status != http.StatusSeeOther {
		t.Fatalf("a person who is not signed in is sent to sign in: %d", status)
	}
	token := link[strings.Index(link, "t=")+2:]
	if status, _, _ := b.post("/signin/link", url.Values{"t": {token}}); status != http.StatusSeeOther {
		t.Fatal(status)
	}
	if status, page := b.get("/org"); status != 200 || !strings.Contains(page, "Acme") {
		t.Fatalf("%d", status)
	}
	if status, _, _ := f.browser().post("/signin/link", url.Values{"t": {token}}); status != http.StatusGone {
		t.Errorf("the link works once: %d", status)
	}
	// A link made to verify an address does not sign anyone in.
	verify := f.mails()[0]
	vt := linkInMail.FindStringSubmatch(verify)[1]
	if status, _, _ := f.browser().post("/signin/link", url.Values{"t": {vt[strings.Index(vt, "t=")+2:]}}); status != http.StatusGone {
		t.Errorf("a verification link is for verifying: %d", status)
	}
	b.post("/signout", url.Values{})
	if status, _ := b.get("/org"); status != http.StatusSeeOther {
		t.Errorf("signing out ends the session: %d", status)
	}
}

// Nobody outside an organisation sees it: another person's page names another
// organisation, and a cookie made up for an address is no sign-in.
func TestOnePersonNeverSeesAnotherOrganisation(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	ada, bob := f.browser(), f.browser()
	ada.post("/signup", signup("ada@acme.com", "Acme Research"))
	bob.post("/signup", signup("bob@globex.com", "Globex Secret Works"))

	_, adaPage := ada.get("/org")
	_, bobPage := bob.get("/org")
	if strings.Contains(adaPage, "Globex") || strings.Contains(adaPage, "bob@globex.com") || strings.Contains(bobPage, "Acme") || strings.Contains(bobPage, "ada@acme.com") {
		t.Errorf("a page named the other organisation:\n%s\n%s", adaPage, bobPage)
	}
	// The page takes nothing from the request to say which organisation it draws.
	if _, page := ada.get("/org?org=globex-secret-works&slug=globex-secret-works"); strings.Contains(page, "Globex") {
		t.Error("a parameter chose the organisation")
	}
	// A cookie for another address, made with the wrong key, is nobody.
	forged := f.browser()
	other, _ := OpenSessions(t.TempDir(), false)
	recorder := httptest.NewRecorder()
	other.Issue(recorder, "bob@globex.com")
	u, _ := url.Parse(f.http.URL)
	forged.client.Jar.SetCookies(u, recorder.Result().Cookies())
	if status, _ := forged.get("/org"); status != http.StatusSeeOther {
		t.Errorf("a cookie signed with another key signs nobody in: %d", status)
	}
	// And the organisations' directories are apart.
	if f.store.DataDir("acme-com") == f.store.DataDir("globex-com") {
		t.Error("two organisations share a directory")
	}
}

func TestTheLinkToVerifyCanBeAskedForAgainUpToTheMailboxLimit(t *testing.T) {
	f := newFront(t, Settings{}, func(c *Config) { c.PerMailbox = 2 })
	b := f.browser()
	b.post("/signup", signup("ada@acme.com", "Acme"))
	if status, _, _ := b.post("/org/verify", url.Values{}); status != http.StatusSeeOther {
		t.Errorf("the first request for another link: %d", status)
	}
	if status, _, _ := b.post("/org/verify", url.Values{}); status != http.StatusSeeOther {
		t.Errorf("the second: %d", status)
	}
	if status, page, _ := b.post("/org/verify", url.Values{}); status != http.StatusTooManyRequests || !strings.Contains(page, "mailed too often") {
		t.Errorf("the third is over the limit: %d", status)
	}
	if got := len(f.mails()); got != 3 {
		t.Errorf("%d mails were sent, and the sign-up's and two more make three", got)
	}
}

func TestEveryPageIsSentWithTheHeadersThatKeepItFromBeingFramedOrScripted(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	_, _, header := f.browser().do("GET", "/", nil, nil)
	for name, want := range map[string]string{"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Cache-Control": "no-store"} {
		if header.Get(name) != want {
			t.Errorf("%s = %q, want %q", name, header.Get(name), want)
		}
	}
	csp := header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || strings.Contains(csp, "unsafe-inline") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("the policy allows too much: %q", csp)
	}
	if status, _, h := f.browser().do("GET", "/static/cloud.css", nil, nil); status != 200 || !strings.Contains(h.Get("Cache-Control"), "max-age") {
		t.Errorf("the stylesheet: %d %v", status, h)
	}
	if status, _, _ := f.browser().do("GET", "/static/tokens.css", nil, nil); status != 200 {
		t.Errorf("the design tokens: %d", status)
	}
}

func TestTheFormAsksForOnlyAWorkEmailAndTheCompanyNamesTheOrganisation(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	if _, page := f.browser().get("/"); strings.Contains(page, `name="org"`) || strings.Contains(page, `name="slug"`) || !strings.Contains(page, `name="email"`) {
		t.Errorf("the form should ask for the email and nothing else:\n%s", page)
	}
	b := f.browser()
	if status, _, _ := b.post("/signup", url.Values{"email": {"ada@acme-research.com"}}); status != http.StatusSeeOther {
		t.Fatal(status)
	}
	if _, page := b.get("/org"); !strings.Contains(page, "Acme Research") || !strings.Contains(page, "acme-research-com.cloud.test") || !strings.Contains(page, "acme-research.com") {
		t.Errorf("the domain gave the name and the address, and the page shows the domain:\n%s", page)
	}
}

func TestAFreeMailSignUpIsAskedForANameAndKeepsTheEmailTyped(t *testing.T) {
	f := newFront(t, Settings{AllowFreeMail: true}, nil)
	status, page, _ := f.browser().post("/signup", url.Values{"email": {"ada@gmail.com"}})
	if status != http.StatusUnprocessableEntity || !strings.Contains(page, "give your organisation a name") || !strings.Contains(page, `name="org"`) || !strings.Contains(page, `value="ada@gmail.com"`) {
		t.Fatalf("%d\n%s", status, page)
	}
	b := f.browser()
	if status, _, _ := b.post("/signup", url.Values{"email": {"ada@gmail.com"}, "org": {"Ada's Lab"}}); status != http.StatusSeeOther {
		t.Errorf("%d", status)
	}
	if _, page := b.get("/org"); !strings.Contains(page, "Ada&#39;s Lab") || !strings.Contains(page, "ada-s-lab.cloud.test") {
		t.Errorf("%s", page)
	}
}

// Two companies whose domains differ by a suffix have the same name and each has an
// address of its own. Neither is put ahead of the other for arriving first, and
// each page shows the domain, which is what tells them apart.
func TestTwoCompaniesWithAlikeNamesAreTreatedTheSame(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	com, co := f.browser(), f.browser()
	for _, who := range []struct {
		b     *browser
		email string
	}{{com, "ada@acme-research.com"}, {co, "bob@acme-research.co"}} {
		if status, _, _ := who.b.post("/signup", url.Values{"email": {who.email}}); status != http.StatusSeeOther {
			t.Fatalf("%s: %d", who.email, status)
		}
	}
	_, comPage := com.get("/org")
	_, coPage := co.get("/org")
	for page, want := range map[string][]string{comPage: {"Acme Research", "acme-research-com.cloud.test", "acme-research.com"}, coPage: {"Acme Research", "acme-research-co.cloud.test", "acme-research.co"}} {
		for _, text := range want {
			if !strings.Contains(page, text) {
				t.Errorf("a page lacks %q:\n%s", text, page)
			}
		}
	}
	if strings.Contains(comPage, "acme-research-co.") || strings.Contains(coPage, "acme-research-com.") {
		t.Error("a page shows the other company's address")
	}
	// An address chosen on purpose that is taken is refused, and the form asks for another.
	status, page, _ := f.browser().post("/signup", url.Values{"email": {"carol@example.org"}, "org": {"Carol"}, "slug": {"acme-research-com"}})
	if status != http.StatusConflict || !strings.Contains(page, "is taken") || !strings.Contains(page, `name="slug"`) {
		t.Errorf("%d\n%s", status, page)
	}
}

// A company with subsidiaries has an organisation for each. The second at a domain is
// asked what to call it, and then has an address from that name.
func TestACompanyWithSubsidiariesHasAnOrganisationForEach(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	first, second := f.browser(), f.browser()
	if status, _, _ := first.post("/signup", url.Values{"email": {"ada@acme.com"}}); status != http.StatusSeeOther {
		t.Fatal(status)
	}
	status, page, _ := second.post("/signup", url.Values{"email": {"bob@acme.com"}})
	if status != http.StatusUnprocessableEntity || !strings.Contains(page, "name this one") || !strings.Contains(page, `name="org"`) || !strings.Contains(page, `value="bob@acme.com"`) {
		t.Fatalf("the second is asked for a name and keeps the email:\n%d %s", status, page)
	}
	if status, _, _ := second.post("/signup", url.Values{"email": {"bob@acme.com"}, "org": {"Acme Insurance"}}); status != http.StatusSeeOther {
		t.Fatal(status)
	}
	_, firstPage := first.get("/org")
	_, secondPage := second.get("/org")
	for page, want := range map[string][]string{firstPage: {"acme-com.cloud.test", "ada@acme.com", "acme.com"}, secondPage: {"Acme Insurance", "acme-insurance.cloud.test", "bob@acme.com", "acme.com"}} {
		for _, text := range want {
			if !strings.Contains(page, text) {
				t.Errorf("a page lacks %q:\n%s", text, page)
			}
		}
	}
	if strings.Contains(firstPage, "Insurance") || strings.Contains(secondPage, "ada@acme.com") {
		t.Error("one organisation's page names the other")
	}
}
