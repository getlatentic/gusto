package cloud

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// withServers is a front door that starts an organisation's server, the fake, for
// each sign-up.
func withServers(t *testing.T, extra ...string) *front {
	t.Helper()
	return newFront(t, Settings{PerDomain: 100}, func(c *Config) { c.Controller = controlled(t, c.Store, extra...) })
}

func (b *browser) at(host, method, path string, header map[string]string) (int, string, http.Header) {
	b.f.t.Helper()
	merged := map[string]string{"Host": host}
	for k, v := range header {
		merged[k] = v
	}
	return b.do(method, path, nil, merged)
}

func (f *front) signedUp(email string) *browser {
	f.t.Helper()
	b := f.browser()
	if status, _, _ := b.post("/signup", url.Values{"email": {email}}); status != http.StatusSeeOther {
		f.t.Fatalf("%s: %d", email, status)
	}
	f.server.Wait()
	return b
}

func TestSigningUpStartsTheOrganisationsServerAndThePageOffersToOpenIt(t *testing.T) {
	f := withServers(t)
	b := f.signedUp("ada@acme.com")
	_, page := b.get("/org")
	if !strings.Contains(page, "Ready") || !strings.Contains(page, `href="/open"`) || strings.Contains(page, "http-equiv") {
		t.Errorf("the server is ready and can be opened:\n%s", page)
	}
}

func TestWhileTheServerIsStartingThePageSaysSoAndLooksAgain(t *testing.T) {
	f := withServers(t, "FAKE_DELAY=1500ms")
	b := f.browser()
	b.post("/signup", url.Values{"email": {"ada@acme.com"}})
	_, page := b.get("/org")
	if !strings.Contains(page, "Starting") || !strings.Contains(page, `http-equiv="refresh"`) || strings.Contains(page, `href="/open"`) {
		t.Errorf("%s", page)
	}
	if status, _, header := b.get2("/open"); status != http.StatusSeeOther || header.Get("Location") != "/org" {
		t.Errorf("there is nothing to open yet: %d %v", status, header)
	}
	f.server.Wait()
	if _, page := b.get("/org"); !strings.Contains(page, "Ready") {
		t.Errorf("%s", page)
	}
}

func (b *browser) get2(path string) (int, string, http.Header) { return b.do("GET", path, nil, nil) }

// The whole way in: the person is signed in at the front door, is sent to their
// organisation's own address with a token, and the organisation's server signs them in
// from it. Nothing about the organisation is asked of them again.
func TestOpeningAnOrganisationSignsThePersonInToItsServer(t *testing.T) {
	f := withServers(t)
	b := f.signedUp("ada@acme.com")
	status, _, header := b.get2("/open")
	location := header.Get("Location")
	if status != http.StatusSeeOther || !strings.HasPrefix(location, "http://acme-com.cloud.test/auth/entry?t=") {
		t.Fatalf("%d %q", status, location)
	}
	parsed, _ := url.Parse(location)
	status, _, answer := b.at(parsed.Host, "GET", parsed.RequestURI(), nil)
	if status != http.StatusSeeOther || answer.Get("Location") != "/" || !strings.Contains(strings.Join(answer.Values("Set-Cookie"), ";"), "haas=ada@acme.com|admin") {
		t.Errorf("the organisation's server signs them in as who and what they are: %d %v", status, answer)
	}
	if status, _, header := f.browser().get2("/open"); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
		t.Errorf("a stranger cannot open anything: %d %v", status, header)
	}
}

func TestEachHostNameIsOneOrganisationAndTheFrontDoorIsTheDomainItself(t *testing.T) {
	f := withServers(t)
	b := f.signedUp("ada@acme.com")
	status, body, _ := b.at("acme-com.cloud.test", "GET", "/whoami", nil)
	var said map[string]any
	json.Unmarshal([]byte(body), &said)
	if status != 200 || said["org"] != "acme-com" || said["host"] != "acme-com.cloud.test" {
		t.Errorf("%d %s", status, body)
	}
	if status, _, _ := b.at("nobody.cloud.test", "GET", "/whoami", nil); status != http.StatusNotFound {
		t.Errorf("no organisation of that name: %d", status)
	}
	if status, page, _ := b.at("cloud.test", "GET", "/signin", nil); status != 200 || !strings.Contains(page, "Email me a link") {
		t.Errorf("the domain itself is the front door: %d", status)
	}
	if status, body, _ := b.at("a.acme-com.cloud.test", "GET", "/whoami", nil); status == 200 && strings.Contains(body, "acme-com") {
		t.Error("a name with two labels is not an organisation")
	}
	if status, _, _ := b.at("ACME-COM.CLOUD.TEST:8050", "GET", "/whoami", nil); status != 200 {
		t.Errorf("a host name is not case-sensitive and may have a port: %d", status)
	}
}

func TestTheFrontDoorSaysWhoTheClientIsAndKeepsItsOwnCookiesToItself(t *testing.T) {
	f := withServers(t)
	b := f.signedUp("ada@acme.com")
	_, body, _ := b.at("acme-com.cloud.test", "GET", "/whoami", map[string]string{
		"X-Forwarded-For": "6.6.6.6", "X-Forwarded-Proto": "https", "Cookie": "haas_cloud=abc; other=1; __Host-haas_cloud=x; haas=session",
	})
	var said map[string]any
	json.Unmarshal([]byte(body), &said)
	if said["xff"] != "127.0.0.1" || said["proto"] != "http" {
		t.Errorf("what the client claimed is not believed, since the server writes down where people signed in from: %v %v", said["xff"], said["proto"])
	}
	if said["cookie"] != "other=1; haas=session" {
		t.Errorf("the front door's cookies do not go to an organisation's server: %q", said["cookie"])
	}

	behind := newFront(t, Settings{}, func(c *Config) { c.TrustProxy = true; c.Controller = controlled(t, c.Store) })
	behind.signedUp("ada@acme.com")
	_, body, _ = behind.browser().at("acme-com.cloud.test", "GET", "/whoami", map[string]string{"X-Forwarded-For": "6.6.6.6, 203.0.113.9"})
	json.Unmarshal([]byte(body), &said)
	if said["xff"] != "203.0.113.9" {
		t.Errorf("behind a proxy the address is the one it wrote: %v", said["xff"])
	}
}

func TestAStreamFromAnOrganisationsServerArrivesAsItIsWritten(t *testing.T) {
	f := withServers(t)
	f.signedUp("ada@acme.com")
	request, _ := http.NewRequest("GET", f.http.URL+"/stream", nil)
	request.Host = "acme-com.cloud.test"
	began := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	lines := bufio.NewScanner(response.Body)
	lines.Scan()
	first := time.Since(began)
	for lines.Scan() {
	}
	if total := time.Since(began); first > 250*time.Millisecond || total < 300*time.Millisecond {
		t.Errorf("the first line came after %s of %s: the stream was held and sent at the end", first, total)
	}
}

// One organisation's token is good in that organisation and in no other: each has a
// key of its own, and the token names its organisation.
func TestAPersonCannotEnterAnotherOrganisationWithTheirOwnsToken(t *testing.T) {
	f := withServers(t)
	ada := f.signedUp("ada@acme.com")
	bob := f.signedUp("bob@globex.com")
	_, _, header := ada.get2("/open")
	parsed, _ := url.Parse(header.Get("Location"))
	if status, _, _ := ada.at("globex-com.cloud.test", "GET", parsed.RequestURI(), nil); status != http.StatusForbidden {
		t.Errorf("Acme's token was accepted by Globex: %d", status)
	}
	// And what each is told of itself is its own.
	for who, host := range map[*browser]string{ada: "acme-com.cloud.test", bob: "globex-com.cloud.test"} {
		_, body, _ := who.at(host, "GET", "/whoami", nil)
		var said map[string]any
		json.Unmarshal([]byte(body), &said)
		if !strings.Contains(host, strings.TrimSuffix(said["org"].(string), "")) {
			t.Errorf("%s answered as %v", host, said["org"])
		}
	}
	_, _, header = bob.get2("/open")
	if !strings.HasPrefix(header.Get("Location"), "http://globex-com.cloud.test/") {
		t.Errorf("a person opens their own organisation: %q", header.Get("Location"))
	}
}

func TestAnOrganisationThatIsNotReadyIsToldToWaitAndNotSentAnError(t *testing.T) {
	f := withServers(t, "FAKE_DELAY=1500ms")
	b := f.browser()
	b.post("/signup", url.Values{"email": {"ada@acme.com"}})
	status, body, header := b.at("acme-com.cloud.test", "GET", "/", nil)
	if status != http.StatusServiceUnavailable || header.Get("Retry-After") == "" || !strings.Contains(body, "http-equiv=\"refresh\"") {
		t.Errorf("%d %v\n%s", status, header, body)
	}
	f.server.Wait()
}

func TestWithNoControllerTheFrontDoorRoutesNothing(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	status, page, _ := f.browser().at("acme-com.cloud.test", "GET", "/", nil)
	if status != 200 || !strings.Contains(page, "Create your organisation") {
		t.Errorf("%d", status)
	}
}
