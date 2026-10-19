package cloud

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var noon = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// leased is a lease that starts with the given time left and may never run past hardMax from noon.
func leased(t *testing.T, left, hardMax time.Duration) (*Lease, string, *time.Time) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lease.json")
	now := noon
	l := OpenLease(path, 30*time.Minute)
	l.now = func() time.Time { return now }
	if err := writeFile(path, leaseFile{Deadline: noon.Add(left).Unix(), HardMax: noon.Add(hardMax).Unix()}, 0o644); err != nil {
		t.Fatal(err)
	}
	return l, path, &now
}

func TestALeaseSaysHowLongIsLeftAndNeverLessThanNone(t *testing.T) {
	l, _, now := leased(t, 20*time.Minute, 4*time.Hour)
	left, err := l.Status()
	if err != nil || left.Remaining != 20*time.Minute || !left.CanExtend {
		t.Fatalf("%+v %v", left, err)
	}
	*now = noon.Add(time.Hour)
	if left, _ := l.Status(); left.Remaining != 0 {
		t.Errorf("a lease that ran out has %v left", left.Remaining)
	}
}

func TestExtendingAddsAStepToWhatIsLeftAndNeverPastTheLatestTheHostMayGo(t *testing.T) {
	l, _, now := leased(t, 20*time.Minute, 90*time.Minute)
	if left, err := l.Extend(); err != nil || left.Remaining != 50*time.Minute {
		t.Fatalf("what is left and a step: %+v %v", left, err)
	}
	if left, err := l.Extend(); err != nil || left.Remaining != 80*time.Minute {
		t.Fatalf("%+v %v", left, err)
	}
	left, err := l.Extend()
	if err != nil || left.Remaining != 90*time.Minute || left.CanExtend {
		t.Fatalf("the last extension stops at the latest: %+v %v", left, err)
	}
	if _, err := l.Extend(); !errors.Is(err, ErrLeaseAtMaximum) {
		t.Errorf("past the latest: %v", err)
	}
	// A lease that has run out is extended from now, and not from a time that has passed.
	l2, _, now2 := leased(t, 5*time.Minute, 5*time.Hour)
	*now2 = noon.Add(2 * time.Hour)
	if left, err := l2.Extend(); err != nil || left.Remaining != 30*time.Minute {
		t.Errorf("%+v %v", left, err)
	}
	_ = now
}

func TestAHostWithNoLeaseFileHasNoLeaseAndAFileThatIsNotOneIsRefused(t *testing.T) {
	l := OpenLease(filepath.Join(t.TempDir(), "none.json"), time.Minute)
	if _, err := l.Status(); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte("not json"), 0o644)
	if _, err := OpenLease(bad, time.Minute).Extend(); err == nil {
		t.Error("a file that is not a lease was extended")
	}
}

func leasedFront(t *testing.T, left, hardMax time.Duration) (*front, *Lease) {
	l, _, _ := leased(t, left, hardMax)
	l.now = time.Now
	writeFile(l.path, leaseFile{Deadline: time.Now().Add(left).Unix(), HardMax: time.Now().Add(hardMax).Unix()}, 0o644)
	return newFront(t, Settings{}, func(c *Config) { c.Lease = l }), l
}

func TestEveryPageOfAHostWithALeaseCarriesTheScriptAndAnswersHowLongIsLeft(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, _ := leasedFront(t, 20*time.Minute, 4*time.Hour)
		b := f.browser()
		if _, page := b.get("/"); !strings.Contains(page, `src="/static/lease.js`) || !strings.Contains(page, `data-extend="/lease"`) {
			t.Errorf("the sign-up page carries no script:\n%s", page)
		}
		// Asked on the front door's own name and on an organisation's, which is what its pages ask.
		for _, host := range []string{"cloud.test", "acme.cloud.test", "anything.cloud.test:8443"} {
			status, body, header := b.do("GET", "/.gusto/lease", nil, map[string]string{"Host": host})
			if status != 200 || !strings.HasPrefix(body, `{"seconds":`) || header.Get("Cache-Control") != "no-store" {
				t.Errorf("%s: %d %q", host, status, body)
			}
		}
	})
}

func TestAHostWithNoLeaseSaysSoAndCarriesNoScript(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		b := f.browser()
		if _, page := b.get("/"); strings.Contains(page, "lease.js") {
			t.Error("a host with no lease carries the script")
		}
		for _, path := range []string{"/.gusto/lease", "/lease"} {
			if status, _ := b.get(path); status != http.StatusNotFound && status != http.StatusSeeOther {
				t.Errorf("%s: %d", path, status)
			}
		}
		b.post("/signup", signup("ada@acme.com", "Acme"))
		if status, _ := b.get("/lease"); status != http.StatusNotFound {
			t.Errorf("signed in, /lease: %d", status)
		}
		if status, _, _ := b.post("/lease/extend", nil); status != http.StatusNotFound {
			t.Errorf("extending: %d", status)
		}
	})
}

func TestOnlyAPersonSignedInExtendsAndTheHostKeepsItsLatestEnd(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, l := leasedFront(t, 10*time.Minute, 50*time.Minute)
		stranger := f.browser()
		if status, _, header := stranger.post("/lease/extend", nil); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
			t.Errorf("a stranger extended: %d %v", status, header)
		}
		if left, _ := l.Status(); left.Remaining > 10*time.Minute {
			t.Errorf("a stranger's request changed the lease: %v", left.Remaining)
		}
		person := f.browser()
		person.post("/signup", signup("ada@acme.com", "Acme"))
		if status, page := person.get("/lease"); status != 200 || !strings.Contains(page, "Keep it for 30 minutes more") {
			t.Fatalf("%d\n%s", status, page)
		}
		if status, _, header := person.post("/lease/extend", nil); status != http.StatusSeeOther || header.Get("Location") != "/lease" {
			t.Fatalf("%d %v", status, header)
		}
		if left, _ := l.Status(); left.Remaining < 39*time.Minute || left.Remaining > 40*time.Minute {
			t.Errorf("ten minutes and a step: %v", left.Remaining)
		}
		person.post("/lease/extend", nil)
		if status, page, _ := person.do("POST", "/lease/extend", nil, nil); status != http.StatusConflict || !strings.Contains(page, "as long as it may have") {
			t.Errorf("past the latest: %d\n%s", status, page)
		}
		if left, _ := l.Status(); left.Remaining > 50*time.Minute {
			t.Errorf("the host was kept past its latest end: %v", left.Remaining)
		}
	})
}
