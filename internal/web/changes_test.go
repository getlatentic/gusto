package web

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/signing"
)

// recording is a site whose changes are written to a log signed by a key the
// site's keyring holds.
func recording(t *testing.T) (*site, *changelog.Log) {
	t.Helper()
	here := open(t)
	ring := t.TempDir()
	key, err := signing.Generate(ring, "acme")
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	log, err := changelog.Open(filepath.Join(t.TempDir(), "changes.jsonl"), key, keyring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	here.server.UseKeyring(keyring)
	here.server.UseChanges(log)
	return here, log
}

func written(t *testing.T, log *changelog.Log) []changelog.Event {
	t.Helper()
	file, err := os.Open(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var events []changelog.Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var e changelog.Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	return events
}

func ofKind(events []changelog.Event, kind string) []changelog.Event {
	var found []changelog.Event
	for _, e := range events {
		if e.Kind == kind {
			found = append(found, e)
		}
	}
	return found
}

func TestSigningInAndBeingRefusedAreRecordedAndTheTypedPasswordIsNot(t *testing.T) {
	here, log := recording(t)
	typed := "a-guess-nobody-should-keep"

	here.ask(t, "POST", "/login", url.Values{"password": {typed}})
	here.signIn(t)

	events := written(t, log)
	refused := ofKind(events, changelog.Refused)
	signedIn := ofKind(events, changelog.SignedIn)
	if len(refused) != 1 || len(signedIn) != 1 {
		t.Fatalf("%d refusals and %d sign-ins in %+v", len(refused), len(signedIn), events)
	}
	if refused[0].What["how"] != "the shared password" || refused[0].From == "" || refused[0].At == "" {
		t.Errorf("the refusal: %+v", refused[0])
	}
	if signedIn[0].Who != sharedSignIn || signedIn[0].Role != string(Admin) {
		t.Errorf("the sign-in: %+v", signedIn[0])
	}
	text, err := os.ReadFile(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), typed) || strings.Contains(string(text), password) {
		t.Error("a password is in the record")
	}
}

func TestSigningOutIsRecordedWithWhoDidIt(t *testing.T) {
	here, log := recording(t)
	here.server.UseAdmins([]string{"admin@example.com"})
	admin := here.as(t, "admin@example.com")

	admin.ask(t, "POST", "/logout", url.Values{})

	out := ofKind(written(t, log), changelog.SignedOut)
	if len(out) != 1 || out[0].Who != "admin@example.com" || out[0].Role != string(Admin) {
		t.Fatalf("%+v", out)
	}
}

func TestEverySetupAndSkillChangeIsRecordedWithWhoMadeIt(t *testing.T) {
	here, log := recording(t)
	here.server.UseAdmins([]string{"admin@example.com"})
	admin, runner := here.as(t, "admin@example.com"), here.as(t, alice)

	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}})
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}, "server": {"memory"}})
	admin.ask(t, "POST", "/admin/skill-sets/reviewer/skills", url.Values{"skill": {"review"}, "description": {"when"}, "body": {"do"}})
	admin.ask(t, "POST", "/admin/skill-sets/reviewer/skills/review/drop", nil)
	// A refused attempt changes nothing, so it writes nothing.
	runner.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"another"}})

	events := written(t, log)
	saved := ofKind(events, changelog.SetupSaved)
	if len(saved) != 2 || saved[0].What["action"] != "created" || saved[1].What["action"] != "changed" || saved[1].What["servers"] != "memory" {
		t.Fatalf("saved: %+v", saved)
	}
	wrote, dropped := ofKind(events, changelog.SkillWritten), ofKind(events, changelog.SkillDropped)
	if len(wrote) != 1 || wrote[0].What["skill"] != "review" || wrote[0].What["setup"] != "reviewer" {
		t.Errorf("written: %+v", wrote)
	}
	if len(dropped) != 1 || dropped[0].What["skill"] != "review" {
		t.Errorf("dropped: %+v", dropped)
	}
	for _, e := range append(append(saved, wrote...), dropped...) {
		if e.Who != "admin@example.com" || e.Role != string(Admin) || e.At == "" {
			t.Errorf("who and when are missing: %+v", e)
		}
	}
	for _, e := range events {
		if strings.Contains(e.What["setup"], "another") {
			t.Errorf("a refused change was recorded: %+v", e)
		}
	}
}

func TestARoleThatIsNotTheOneSomebodyLastHadIsRecorded(t *testing.T) {
	here, log := recording(t)
	request := httptest.NewRequest("GET", "/auth/callback", nil)

	here.server.noteSignIn(request, "bob@example.com", Runner, "the provider")
	here.server.noteSignIn(request, "bob@example.com", Runner, "the provider")
	here.server.noteSignIn(request, "bob@example.com", Admin, "the provider")

	events := written(t, log)
	changed := ofKind(events, changelog.RoleChanged)
	if len(changed) != 1 || changed[0].What["was"] != "runner" || changed[0].What["now"] != "admin" || changed[0].Who != "bob@example.com" {
		t.Fatalf("%+v", changed)
	}
	if len(ofKind(events, changelog.SignedIn)) != 3 {
		t.Errorf("each sign-in should be recorded: %+v", events)
	}
}

func TestAPersonTheProviderAdmitsButNoRoleMapsToIsRecordedAsRefused(t *testing.T) {
	here, log := recording(t)
	request := httptest.NewRequest("GET", "/auth/callback", nil)

	here.server.noteRefused(request, "carol@example.com", "the provider", "no role maps to this person")

	refused := ofKind(written(t, log), changelog.Refused)
	if len(refused) != 1 || refused[0].Who != "carol@example.com" || refused[0].What["why"] != "no role maps to this person" {
		t.Fatalf("%+v", refused)
	}
}

// Anybody can be refused, so the record cannot be a way to fill the disk: a
// source past its limit has the rest counted, and the count is written with the
// next one that is.
func TestARefusalFloodIsCountedAndNotWritten(t *testing.T) {
	var f refusals
	start := time.Now()
	written, dropped := 0, 0
	for i := 0; i < 25; i++ {
		if ok, _ := f.allow("203.0.113.9", start.Add(time.Duration(i)*time.Second)); ok {
			written++
		} else {
			dropped++
		}
	}
	if written != refusalsPerMinute || dropped != 25-refusalsPerMinute {
		t.Fatalf("%d written and %d dropped", written, dropped)
	}
	if ok, _ := f.allow("198.51.100.4", start); !ok {
		t.Error("another source was limited by the first")
	}
	ok, carried := f.allow("203.0.113.9", start.Add(3*refusalWindow))
	if !ok || carried != 25-refusalsPerMinute {
		t.Errorf("after the window: allowed %v carrying %d, want %d", ok, carried, 25-refusalsPerMinute)
	}
}

func TestTheSettingsAServerStartsWithAreRecordedAndEveryChangeToThem(t *testing.T) {
	_, log := recording(t)
	first := &Server{changes: log}

	first.UseSettings(map[string]string{"GUSTO_ADMINS": "a@example.com", "GUSTO_ALLOWED": "@example.com"})
	first.UseSettings(map[string]string{"GUSTO_ADMINS": "a@example.com", "GUSTO_ALLOWED": "@example.com"})
	first.UseSettings(map[string]string{"GUSTO_ADMINS": "a@example.com,b@example.com", "GUSTO_ALLOWED": "@example.com"})

	events := written(t, log)
	if starts := ofKind(events, changelog.ServerStarted); len(starts) != 3 {
		t.Fatalf("%d starts", len(starts))
	}
	changed := ofKind(events, changelog.SettingsChanged)
	if len(changed) != 1 || changed[0].What["setting"] != "GUSTO_ADMINS" ||
		changed[0].What["was"] != "a@example.com" || changed[0].What["now"] != "a@example.com,b@example.com" {
		t.Fatalf("the change: %+v", changed)
	}
}

func TestOnlyAnAdminSeesTheChangesAndCanDownloadThem(t *testing.T) {
	here, log := recording(t)
	here.server.UseAdmins([]string{"admin@example.com"})
	admin, runner := here.as(t, "admin@example.com"), here.as(t, alice)
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}})

	page := admin.ask(t, "GET", "/admin/records", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("an admin got %d", page.Code)
	}
	for _, want := range []string{"created skill set reviewer", "admin@example.com", "the chain holds", "gusto changes verify"} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("the page does not say %q:\n%s", want, page.Body.String())
		}
	}
	download := admin.ask(t, "GET", "/admin/activity.jsonl", nil)
	if download.Code != http.StatusOK || !strings.Contains(download.Header().Get("Content-Disposition"), "changes.jsonl") {
		t.Fatalf("the download: %d %v", download.Code, download.Header())
	}
	report, err := changelog.Verify(strings.NewReader(download.Body.String()), here.server.keyring)
	if err != nil || report.Events < 1 || report.Signers["acme"] != report.Events {
		t.Errorf("what was downloaded does not verify: %+v %v", report, err)
	}
	if signed := eventsSaid(report.Events, report.Signed); report.Signed != report.Events || !strings.Contains(page.Body.String(), signed) {
		t.Errorf("the page does not count the signed lines as %q", signed)
	}

	for _, path := range []string{"/admin/records", "/admin/activity.jsonl"} {
		if got := runner.ask(t, "GET", path, nil); got.Code != http.StatusForbidden {
			t.Errorf("a runner asking for %s got %d", path, got.Code)
		}
	}
	stranger := httptest.NewRecorder()
	here.handler.ServeHTTP(stranger, httptest.NewRequest("GET", "/admin/records", nil))
	if stranger.Code != http.StatusSeeOther {
		t.Errorf("somebody not signed in got %d", stranger.Code)
	}
	_ = log
}

func TestAScriptWithThePasswordReadsTheSameRecord(t *testing.T) {
	here, _ := recording(t)
	here.signIn(t)
	script := httptest.NewRequest("GET", "/api/changes", nil)
	script.Header.Set("Authorization", "Bearer "+password)
	answer := httptest.NewRecorder()
	here.handler.ServeHTTP(answer, script)

	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"kind":"signin"`) {
		t.Fatalf("%d %s", answer.Code, answer.Body.String())
	}
	none := httptest.NewRecorder()
	here.handler.ServeHTTP(none, httptest.NewRequest("GET", "/api/changes", nil))
	if none.Code != http.StatusUnauthorized {
		t.Errorf("a script with no password got %d", none.Code)
	}
}

func TestWithNoLogNothingIsRecordedAndNobodyIsShownAPage(t *testing.T) {
	here := open(t)
	here.signIn(t)
	page := here.ask(t, "GET", "/admin/records", nil).Body.String()
	if strings.Contains(page, "tab=activity") || strings.Contains(page, "/admin/activity.jsonl") {
		t.Errorf("with no log the records offer its activity:\n%s", page)
	}
	if got := here.ask(t, "GET", "/admin/activity.jsonl", nil); got.Code != http.StatusNotFound {
		t.Errorf("with no log the download answered %d", got.Code)
	}
}

// A forwarded address is believed only from the proxy on this machine, because
// anybody can write the header, and then it is the one the proxy added, the last.
func TestAForwardedAddressIsBelievedOnlyFromTheLocalProxy(t *testing.T) {
	behind := httptest.NewRequest("GET", "/", nil)
	behind.RemoteAddr = "127.0.0.1:5000"
	behind.Header.Set("X-Forwarded-For", "10.0.0.2, 203.0.113.7")
	if got := clientOf(behind); got != "203.0.113.7" {
		t.Errorf("behind the proxy: %q", got)
	}
	direct := httptest.NewRequest("GET", "/", nil)
	direct.RemoteAddr = "198.51.100.9:5000"
	direct.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := clientOf(direct); got != "198.51.100.9" {
		t.Errorf("direct: %q, and the header was believed", got)
	}
}

// A run of the same thing from the same place is one row that says how many times, so twelve restarts are not twelve lines.
func TestRepeatedEventsAreOneRowThatCountsThem(t *testing.T) {
	rows := foldRepeats([]row{
		{Who: "the server", Said: "the server started", Full: "newest", Times: 1},
		{Who: "the server", Said: "the server started", Full: "older", Times: 1},
		{Who: "the server", Said: "the server started", Full: "oldest", Times: 1},
		{Who: "ada", Said: "signed out", Times: 1},
		{Who: "the server", Said: "the server started", Times: 1},
	})
	if len(rows) != 3 || rows[0].Times != 3 || rows[0].Since != "oldest" || rows[1].Times != 1 || rows[2].Times != 1 {
		t.Errorf("the rows were %+v", rows)
	}
}

func TestATimeIsAsShortAsItCanBeAndStillClear(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	for when, want := range map[time.Time]string{
		time.Date(2026, 10, 9, 14, 47, 1, 0, time.UTC):  "Today 14:47",
		time.Date(2026, 9, 28, 9, 5, 0, 0, time.UTC):    "28 Sep 09:05",
		time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC): "31 Dec 2025 23:59",
	} {
		if got := shortWhen(when, now); got != want {
			t.Errorf("%v came out as %q, not %q", when, got, want)
		}
	}
}
