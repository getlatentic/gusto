package web

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/inbox"
)

// withInbox is a server with a change record and an inbox the front door leaves events in.
func withInbox(t *testing.T) (*site, *changelog.Log, string) {
	t.Helper()
	here, log := recording(t)
	dir := filepath.Join(t.TempDir(), "inbox")
	here.server.UseInbox(dir)
	return here, log, dir
}

func adminDid(kind string, at time.Time, what map[string]string) inbox.Item {
	return inbox.Item{At: at, Who: "ada@acme.com", Role: "admin", From: "203.0.113.9", Kind: kind, What: what}
}

func drop(t *testing.T, dir string, items ...inbox.Item) {
	t.Helper()
	for _, item := range items {
		if err := inbox.Drop(dir, item); err != nil {
			t.Fatal(err)
		}
	}
}

func inboxFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestTheServerTakesWhatTheFrontDoorReportsIntoItsRecordInOrderAndOnce(t *testing.T) {
	here, log, dir := withInbox(t)
	first := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	drop(t, dir,
		adminDid(changelog.MemberInvited, first, map[string]string{"email": "bob@acme.com", "role": "runner"}),
		adminDid(changelog.MemberRole, first.Add(time.Minute), map[string]string{"email": "bob@acme.com", "was": "runner", "now": "admin"}),
		adminDid(changelog.MemberRemoved, first.Add(2*time.Minute), map[string]string{"email": "bob@acme.com", "role": "admin"}),
	)
	if got := here.server.TakeInbox(); got != 3 {
		t.Fatalf("took %d", got)
	}
	var kinds []string
	events := written(t, log)
	for _, event := range events {
		if strings.HasPrefix(event.Kind, "member.") {
			kinds = append(kinds, event.Kind)
		}
	}
	if strings.Join(kinds, " ") != "member.invited member.role member.removed" {
		t.Fatalf("the record has %v", kinds)
	}
	got := ofKind(events, changelog.MemberRole)[0]
	if got.Who != "ada@acme.com" || got.Role != "admin" || got.From != "203.0.113.9" || got.What["was"] != "runner" || got.What["now"] != "admin" {
		t.Errorf("the event: %+v", got)
	}
	if got.At != first.Add(time.Minute).Format(time.RFC3339Nano) {
		t.Errorf("the record says when it was done and not when it was taken: %s", got.At)
	}
	if files := inboxFiles(t, dir); len(files) != 0 {
		t.Errorf("what was taken is still in the inbox: %v", files)
	}
	if again := here.server.TakeInbox(); again != 0 || len(written(t, log)) != len(events) {
		t.Errorf("a second pass wrote %d more", again)
	}
}

func TestAnEventThatWasWrittenButWhoseFileWasNotRemovedIsNotWrittenAgain(t *testing.T) {
	here, log, dir := withInbox(t)
	drop(t, dir, adminDid(changelog.MemberInvited, time.Now(), map[string]string{"email": "bob@acme.com", "role": "runner"}))
	waiting, _ := inbox.Pending(dir)
	name := filepath.Join(dir, waiting[0].ID+".json")
	kept, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	here.server.TakeInbox()
	if err := os.WriteFile(name, kept, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := here.server.TakeInbox(); got != 0 {
		t.Errorf("an event already in the record was written again: %d", got)
	}
	if n := len(ofKind(written(t, log), changelog.MemberInvited)); n != 1 {
		t.Errorf("the record has the invitation %d times", n)
	}
	if files := inboxFiles(t, dir); len(files) != 0 {
		t.Errorf("the file was left: %v", files)
	}
}

func TestWhatIsNotAnEventTheFrontDoorReportsIsSetAsideAndNeverRecorded(t *testing.T) {
	here, log, dir := withInbox(t)
	drop(t, dir,
		adminDid(changelog.SignedIn, time.Now(), map[string]string{"how": "a forged sign-in"}),
		adminDid(changelog.SettingsChanged, time.Now(), map[string]string{"setting": "GUSTO_ADMINS"}),
	)
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000000-bad.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000001-nobody.json"), []byte(`{"kind":"member.removed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	drop(t, dir, adminDid(changelog.MemberRemoved, time.Now(), map[string]string{"email": "bob@acme.com", "role": "runner"}))

	if got := here.server.TakeInbox(); got != 1 {
		t.Fatalf("took %d", got)
	}
	events := written(t, log)
	if len(ofKind(events, changelog.SignedIn)) != 0 || len(ofKind(events, changelog.SettingsChanged)) != 0 {
		t.Errorf("the front door wrote a kind of event that is the server's own: %+v", events)
	}
	if len(ofKind(events, changelog.MemberRemoved)) != 1 {
		t.Error("the event after the ones that were refused was not taken")
	}
	var aside int
	for _, name := range inboxFiles(t, dir) {
		if strings.HasSuffix(name, ".rejected") {
			aside++
		} else {
			t.Errorf("%s is still where the next pass will meet it", name)
		}
	}
	if aside != 4 {
		t.Errorf("%d files were set aside, four were refused", aside)
	}
}

func TestTheActivityPageShowsWhatTheFrontDoorReportedInWords(t *testing.T) {
	here, _, dir := withInbox(t)
	here.server.UseAdmins([]string{boss})
	admin := here.as(t, boss)
	drop(t, dir,
		adminDid(changelog.MemberInvited, time.Now(), map[string]string{"email": "bob@acme.com", "role": "runner"}),
		adminDid(changelog.MemberRole, time.Now(), map[string]string{"email": "bob@acme.com", "was": "runner", "now": "admin"}),
		adminDid(changelog.MemberRemoved, time.Now(), map[string]string{"email": "bob@acme.com", "role": "admin"}),
	)
	page := admin.ask(t, "GET", "/admin/records", nil).Body.String()
	for _, want := range []string{
		"invited bob@acme.com to join as a runner",
		"changed the role of bob@acme.com from runner to admin",
		"removed bob@acme.com, who was admin",
		"ada@acme.com", "203.0.113.9", "the chain holds",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not say %q:\n%s", want, page)
		}
	}
}

func TestTheServerTakesTheInboxOnItsOwnAsWellAsWhenAPageIsRead(t *testing.T) {
	here, log, dir := withInbox(t)
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		here.server.RunInbox(ctx, 5*time.Millisecond)
		close(done)
	}()
	drop(t, dir, adminDid(changelog.MemberRemoved, time.Now(), map[string]string{"email": "bob@acme.com", "role": "runner"}))
	deadline := time.Now().Add(5 * time.Second)
	for len(ofKind(written(t, log), changelog.MemberRemoved)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("nothing was taken with nobody reading a page")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	<-done
}

func TestAnEventThatCannotBeWrittenStaysInTheInboxForTheNextPass(t *testing.T) {
	here, log, dir := withInbox(t)
	drop(t, dir, adminDid(changelog.MemberRemoved, time.Now(), map[string]string{"email": "bob@acme.com", "role": "runner"}))
	log.Close()
	if got := here.server.TakeInbox(); got != 0 || len(inboxFiles(t, dir)) != 1 {
		t.Errorf("an event the record did not take was counted or removed: took %d, files %v", got, inboxFiles(t, dir))
	}
}

func TestWithNoChangeRecordTheInboxIsLeftAlone(t *testing.T) {
	here := open(t)
	dir := filepath.Join(t.TempDir(), "inbox")
	here.server.UseInbox(dir)
	drop(t, dir, adminDid(changelog.MemberRemoved, time.Now(), map[string]string{"email": "bob@acme.com"}))
	if got := here.server.TakeInbox(); got != 0 || len(inboxFiles(t, dir)) != 1 {
		t.Errorf("events were dropped with nowhere to write them: took %d, files %v", got, inboxFiles(t, dir))
	}
}
