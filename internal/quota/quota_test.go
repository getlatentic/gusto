package quota

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestARefusalSaysWhatWasAllowedAndHowTheLimitIsLifted(t *testing.T) {
	trial := Quota{Jobs: 3, Concurrent: 1, Note: "Confirm your email address to raise this limit."}
	if err := trial.Refuse(2); err != nil {
		t.Fatalf("under the limit: %v", err)
	}
	err := trial.Refuse(3)
	if !errors.Is(err, ErrExceeded) {
		t.Fatalf("a refusal is ErrExceeded: %v", err)
	}
	if want := "This organisation may start 3 jobs in all and has started 3. Confirm your email address to raise this limit."; err.Error() != want {
		t.Errorf("%q, want %q", err.Error(), want)
	}
	monthly := Quota{Jobs: 200, Monthly: true}
	if err := monthly.Refuse(200); err == nil || !strings.Contains(err.Error(), "200 jobs a month") || strings.HasSuffix(err.Error(), " ") {
		t.Errorf("%v", err)
	}
	if err := (Quota{}).Refuse(0); !errors.Is(err, ErrExceeded) {
		t.Errorf("a quota of none refuses the first job: %v", err)
	}
}

func TestTheQuotaIsReadAgainWhenASecondHasPassedAndNotBefore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := Write(path, Quota{Jobs: 3}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	f := Open(path)
	f.now = func() time.Time { return now }
	if q, err := f.Current(); err != nil || q.Jobs != 3 {
		t.Fatalf("%+v %v", q, err)
	}
	Write(path, Quota{Jobs: 200, Monthly: true})
	if q, _ := f.Current(); q.Jobs != 3 {
		t.Errorf("a file is not read for every question: %+v", q)
	}
	now = now.Add(1100 * time.Millisecond)
	if q, _ := f.Current(); q.Jobs != 200 || !q.Monthly {
		t.Errorf("a limit that was lifted takes effect: %+v", q)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("the quota is private to the server: %v", info.Mode())
	}
}

// A server that was given limits and lost them starts nothing, and does not
// start everything.
func TestAQuotaThatCannotBeReadIsAnErrorAndNotNoLimit(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(filepath.Join(dir, "missing.json")).Current(); err == nil {
		t.Error("a missing file is an error")
	}
	for name, text := range map[string]string{"not json": "{", "negative": `{"jobs": -1}`, "wrong shape": `[1]`} {
		path := filepath.Join(dir, name+".json")
		os.WriteFile(path, []byte(text), 0o600)
		if _, err := Open(path).Current(); err == nil {
			t.Errorf("%s should be an error", name)
		}
	}
}
