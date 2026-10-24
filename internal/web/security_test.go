package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/job"
)

func TestEveryResponseCarriesTheSecurityHeaders(t *testing.T) {
	here := open(t)
	here.signIn(t)
	for _, path := range []string{"/login", "/", "/jobs", "/spend", "/compare", "/nothing-here", "/healthz", "/static/app.css"} {
		got := here.ask(t, "GET", path, nil)
		for name, want := range map[string]string{
			"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Referrer-Policy": "same-origin",
		} {
			if got.Header().Get(name) != want {
				t.Errorf("%s: %s is %q, want %q", path, name, got.Header().Get(name), want)
			}
		}
		csp := got.Header().Get("Content-Security-Policy")
		for _, want := range []string{"frame-ancestors 'none'", "base-uri 'none'", "object-src 'none'", "form-action 'self'", "default-src 'self'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: the policy lacks %q: %s", path, want, csp)
			}
		}
		if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") || strings.Contains(csp, "unsafe-eval") {
			t.Errorf("%s: scripts are not held to this server's own and the nonce: %s", path, csp)
		}
		if strings.HasPrefix(path, "/static/") {
			continue
		}
		if got.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s may be cached: %q", path, got.Header().Get("Cache-Control"))
		}
	}
	secure := httptest.NewRequest("GET", "/login", nil)
	secure.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()
	here.handler.ServeHTTP(recorder, secure)
	if !strings.Contains(recorder.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Error("a response over TLS does not ask for TLS to be kept")
	}
	if plain := here.ask(t, "GET", "/login", nil); plain.Header().Get("Strict-Transport-Security") != "" {
		t.Error("a response over plain http asked for TLS to be kept")
	}
}

var inlineScript = regexp.MustCompile(`<script([^>]*)>`)

// A page may run only the scripts it was drawn with: each inline one carries
// this response's nonce, and no element carries a handler of its own.
func TestEveryInlineScriptCarriesTheResponsesNonceAndNoElementHasAHandler(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	where := site.ask(t, "POST", "/instances", url.Values{"harness": {"first"}}).Header().Get("Location")
	if !strings.Contains(where, "/threads/") {
		t.Fatalf("starting an agent did not open a conversation: %q", where)
	}
	instance := where[:strings.Index(where, "/threads/")]
	pages := []string{"/chat", "/jobs", "/spend", "/compare", "/new", "/setups", "/engines", instance, where}
	seen := map[string]bool{}
	for _, path := range pages {
		got := site.ask(t, "GET", path, nil)
		if got.Code != http.StatusOK {
			t.Errorf("%s gave %d", path, got.Code)
			continue
		}
		csp := got.Header().Get("Content-Security-Policy")
		nonce := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(csp)
		if nonce == nil {
			t.Fatalf("%s: no nonce in %s", path, csp)
		}
		if seen[nonce[1]] {
			t.Errorf("%s: a nonce was used twice", path)
		}
		seen[nonce[1]] = true
		body := got.Body.String()
		for _, tag := range inlineScript.FindAllStringSubmatch(body, -1) {
			if strings.Contains(tag[1], "src=") {
				continue
			}
			if !strings.Contains(tag[1], `nonce="`+nonce[1]+`"`) {
				t.Errorf("%s: an inline script has no nonce, and would not run: <script%s>", path, tag[1])
			}
		}
		if handler := regexp.MustCompile(`\son[a-z]+=`).FindString(body); handler != "" {
			t.Errorf("%s: an element carries a handler the policy refuses:%s", path, handler)
		}
	}
}

func TestARequestThatSendsMoreThanAPageEverDoesIsCutOff(t *testing.T) {
	here := open(t)
	var read int64
	var failed bool
	handler := here.server.secure(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		read, failed = n, err != nil
	}))
	request := httptest.NewRequest("POST", "/anything", strings.NewReader(strings.Repeat("x", maxBody+1024)))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if !failed || read > maxBody {
		t.Errorf("read %d bytes, failed=%v", read, failed)
	}
}

func TestAWrongPasswordIsTriedTenTimesAndThenTheSourceIsTurnedAway(t *testing.T) {
	here := open(t)
	from := func(address string, password string) int {
		request := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"password": {password}}.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.RemoteAddr = address + ":4000"
		recorder := httptest.NewRecorder()
		here.handler.ServeHTTP(recorder, request)
		return recorder.Code
	}
	for i := 0; i < failuresAllowed; i++ {
		if got := from("203.0.113.5", "guess"); got != http.StatusSeeOther {
			t.Fatalf("try %d gave %d", i+1, got)
		}
	}
	if got := from("203.0.113.5", "guess"); got != http.StatusTooManyRequests {
		t.Errorf("the eleventh wrong password gave %d", got)
	}
	if got := from("203.0.113.5", password); got != http.StatusTooManyRequests {
		t.Errorf("the right password from a source that is guessing gave %d, and told it so", got)
	}
	if got := from("203.0.113.99", password); got != http.StatusSeeOther {
		t.Errorf("somebody else was turned away too: %d", got)
	}
}

func TestAWrongBearerIsTriedTenTimesAndThenTheSourceIsTurnedAway(t *testing.T) {
	here := open(t)
	call := func(bearer string) int {
		request := httptest.NewRequest("GET", "/api/spend", nil)
		request.Header.Set("Authorization", "Bearer "+bearer)
		request.RemoteAddr = "198.51.100.7:1"
		recorder := httptest.NewRecorder()
		here.handler.ServeHTTP(recorder, request)
		return recorder.Code
	}
	for i := 0; i < failuresAllowed; i++ {
		if got := call("guess"); got != http.StatusUnauthorized {
			t.Fatalf("try %d gave %d", i+1, got)
		}
	}
	if got := call(password); got != http.StatusTooManyRequests {
		t.Errorf("a source that had guessed ten times was let in: %d", got)
	}
}

func TestSigningOutEndsTheSessionOnTheServerAndNotOnlyInTheBrowser(t *testing.T) {
	here := open(t)
	here.signIn(t)
	copied := *here.cookie
	if here.ask(t, "GET", "/jobs", nil).Code != http.StatusOK {
		t.Fatal("not signed in")
	}
	here.ask(t, "POST", "/logout", nil)
	stolen := *here
	stolen.cookie = &copied
	if got := stolen.ask(t, "GET", "/jobs", nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/login" {
		t.Errorf("a cookie copied before signing out still works: %d", got.Code)
	}
}

func TestARepositoryAddressIsHeldToWhatGitReachesOverAnEncryptedTransport(t *testing.T) {
	here := open(t)
	for _, repo := range []string{"--upload-pack=touch /tmp/x;", "ext::sh -c id", "file:///etc", "/etc", "/var/lib/gusto/sessions/x/workspace", "http://github.com/a/b"} {
		body, _ := jsonSubmission(repo)
		request := httptest.NewRequest("POST", "/api/jobs", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+password)
		recorder := httptest.NewRecorder()
		here.handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("the API took the repository %q: %d %s", repo, recorder.Code, recorder.Body.String())
		}
		here.signIn(t)
		made := here.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "repo": {repo}})
		if !strings.Contains(made.Header().Get("Location"), "refused=") {
			t.Errorf("a person started an agent on %q: %d %q", repo, made.Code, made.Header().Get("Location"))
		}
	}
	here.nothing(t)
	if got := here.server.instances.List(); len(got) != 0 {
		t.Errorf("an agent was made from a refused address: %d", len(got))
	}
}

func jsonSubmission(repo string) (string, error) {
	return `{"spec":{"brief":"b","repo":` + jsonString(repo) + `,"gate":["true"],"harness":"opencode"}}`, nil
}

func jsonString(s string) string {
	out := `"`
	for _, r := range s {
		if r == '"' || r == '\\' {
			out += `\`
		}
		out += string(r)
	}
	return out + `"`
}

// A link in out/ is left there by a harness that could write the record. A page
// that opened it would show what it points at.
func TestALinkInARecordIsNeverShownOrServed(t *testing.T) {
	here := open(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(secret, []byte("the operator's private key"), 0o600)
	spentJob(t, here.index.Root(), "linked", "", priced)
	dir := job.Dir(filepath.Join(here.index.Root(), "linked"))
	jobHash, files, err := evidence.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Job: jobHash, Files: files}); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, dir.OutFile("loot.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, dir.OutFile("transcript.log")); err != nil {
		t.Fatal(err)
	}
	if err := here.index.Add(jobs.Record{ID: "linked", State: jobs.Passed, Brief: "linked", Harness: "fixture", Asked: "amy@acme.test"}); err != nil {
		t.Fatal(err)
	}
	here.signIn(t)
	for _, name := range []string{"loot.txt", "transcript.log"} {
		got := here.ask(t, "GET", "/jobs/linked/files/"+name, nil)
		if got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "private key") {
			t.Errorf("a link to a file outside the job was served as %s: %d", name, got.Code)
		}
	}
	if page := here.ask(t, "GET", "/jobs/linked", nil).Body.String(); strings.Contains(page, "private key") {
		t.Error("the job page shows what a link points at")
	}
	if wrong, _ := evidence.Check(dir); !strings.Contains(strings.Join(wrong, "; "), "loot.txt is not a plain file") {
		t.Errorf("verify does not name the link: %v", wrong)
	}
}
