package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/harness"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/quota"
)

// onlyBuiltIn is a site whose harnesses are the ones every gusto has, so what the
// tests call an unknown harness does not depend on the machine running them.
func onlyBuiltIn(t *testing.T) *site {
	t.Helper()
	t.Setenv("GUSTO_HARNESSES", "")
	return open(t)
}

func (s *site) send(t *testing.T, bearer string, sent Submission) (int, Answer) {
	t.Helper()
	body, err := json.Marshal(sent)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/api/jobs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	var answer Answer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %q", recorder.Body.String())
	}
	return recorder.Code, answer
}

// nothing checks that a refusal left no job and no directory, because a refused
// job that is still on disk comes back as a job at the next restart.
func (s *site) nothing(t *testing.T) {
	t.Helper()
	if held := s.index.List(); len(held) != 0 {
		t.Fatalf("the index holds %d job(s)", len(held))
	}
	left, err := os.ReadDir(s.index.Root())
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("a refused job left %v behind", left)
	}
}

var fixture = harness.Adapter{
	Name: "fixture", Program: "sh", Argv: []string{"-c", "echo done"},
	Needs: harness.NeedsNothing, Stream: harness.Plain,
}

func aJob() job.Spec {
	return job.Spec{
		Brief:   "Make the adder add",
		Repo:    "https://github.com/acme/repo",
		Gate:    []string{"sh", "-c", "test 2 = 2"},
		Harness: "opencode",
	}
}

func TestAJobSentWithoutTheSharedPasswordIsRefusedAndNothingIsCreated(t *testing.T) {
	site := onlyBuiltIn(t)
	for given, says := range map[string]string{
		"":                 "the shared password goes in an Authorization: Bearer header",
		"not the password": "that is not the shared password",
	} {
		code, answer := site.send(t, given, Submission{Spec: aJob()})
		if code != http.StatusUnauthorized {
			t.Errorf("a password of %q gave %d, and the reason %q", given, code, answer.Refused)
		}
		if answer.ID != "" {
			t.Errorf("a password of %q was given job %s", given, answer.ID)
		}
		// A password that is right in the wrong place and one that is simply
		// wrong are different mistakes, and one message for both sends a person
		// to check a header they wrote correctly.
		if answer.Refused != says {
			t.Errorf("a password of %q was refused with %q, and should be %q", given, answer.Refused, says)
		}
	}
	// The pages' cookie is not a credential here, so nothing a browser sends by
	// itself can post a job on a signed-in person's behalf.
	site.signIn(t)
	if withCookie := site.ask(t, "POST", "/api/jobs", nil); withCookie.Code != http.StatusUnauthorized {
		t.Errorf("the sign-in cookie alone gave %d", withCookie.Code)
	}
	site.nothing(t)
}

func TestAJobSentOverHTTPIsTheSameDirectoryTheCommandLineMakes(t *testing.T) {
	site := onlyBuiltIn(t)
	code, answer := site.send(t, password, Submission{Spec: aJob()})
	if code != http.StatusCreated || answer.ID == "" {
		t.Fatalf("code %d, answer %+v", code, answer)
	}
	record, found := site.index.Get(answer.ID)
	if !found {
		t.Fatalf("no job called %q", answer.ID)
	}
	spec, err := job.Read(record.Dir(site.index.Root()))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Brief != "Make the adder add" || spec.Repo != "https://github.com/acme/repo" || spec.Harness != "opencode" {
		t.Fatalf("the spec on disk is %+v", spec)
	}
	if len(spec.Gate) != 3 || spec.Gate[2] != "test 2 = 2" {
		t.Fatalf("the gate on disk is %#v", spec.Gate)
	}
	// What a person left out is filled in on the way to disk, the same as a job
	// made here: a sent job is not a second kind of job.
	if spec.Ref != "HEAD" || spec.Sensitivity != job.Internal || spec.Budget.Turns != 40 || spec.Budget.Seconds != 900 {
		t.Fatalf("a sent job is missing the defaults: %+v", spec)
	}
	select {
	case <-site.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the job was never picked up")
	}
}

func TestAJobMissingAPartIsRefusedAndSaysWhich(t *testing.T) {
	site := onlyBuiltIn(t)
	for _, refusal := range []struct {
		name string
		spec job.Spec
		says string
	}{
		{"no repository", job.Spec{Brief: "do it", Gate: []string{"true"}, Harness: "opencode"}, "this task needs a repository"},
		{"no brief", job.Spec{Repo: "/repo", Gate: []string{"true"}, Harness: "opencode"}, "this task needs a brief"},
		{"no gate", job.Spec{Repo: "/repo", Brief: "do it", Harness: "opencode"}, "this task needs a check"},
		{"a gate of blanks", job.Spec{Repo: "/repo", Brief: "do it", Gate: []string{" "}, Harness: "opencode"}, "this task needs a check"},
		{"no repository and no gate", job.Spec{Brief: "do it", Harness: "opencode"}, "this task needs a repository and a check"},
		{"nothing at all", job.Spec{Harness: "opencode"}, "this task needs a repository, a brief and a check"},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			code, answer := site.send(t, password, Submission{Spec: refusal.spec})
			if code != http.StatusBadRequest {
				t.Fatalf("code %d", code)
			}
			if answer.Refused != refusal.says {
				t.Fatalf("the reason is %q, and should be %q", answer.Refused, refusal.says)
			}
			site.nothing(t)
		})
	}
}

func TestAHarnessThisServerHasNotHeardOfIsRefusedUnlessTheJobCarriesIt(t *testing.T) {
	site := onlyBuiltIn(t)
	spec := aJob()
	spec.Harness = "fixture"

	code, answer := site.send(t, password, Submission{Spec: spec})
	if code != http.StatusBadRequest {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(answer.Refused, "fixture") || !strings.Contains(answer.Refused, "carries no adapter") {
		t.Fatalf("the reason is %q", answer.Refused)
	}
	site.nothing(t)

	code, answer = site.send(t, password, Submission{Spec: spec, Adapter: &fixture})
	if code != http.StatusCreated || answer.ID == "" {
		t.Fatalf("a carried adapter should be enough: code %d, answer %+v", code, answer)
	}
	record, found := site.index.Get(answer.ID)
	if !found {
		t.Fatalf("no job called %q", answer.ID)
	}
	// The runner looks for a carried adapter in the job directory, so that is
	// where this one has to have landed.
	carried, err := harness.Load(record.Dir(site.index.Root()), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if carried.Program != "sh" || len(carried.Argv) != 2 {
		t.Fatalf("the adapter in the job is %+v", carried)
	}
}

func TestARestrictedJobAgainstAModelOutsideTheBuildingIsRefused(t *testing.T) {
	site := onlyBuiltIn(t)
	spec := aJob()
	spec.Harness = "fixture"
	spec.Sensitivity = job.Restricted
	away := fixture
	away.Needs = harness.NeedsKey
	away.Endpoint = "https://models.example.com/v1"

	code, answer := site.send(t, password, Submission{Spec: spec, Adapter: &away})
	if code != http.StatusBadRequest {
		t.Fatalf("code %d, answer %+v", code, answer)
	}
	if !strings.Contains(answer.Refused, "models.example.com") {
		t.Fatalf("the reason has to name the endpoint: %q", answer.Refused)
	}
	site.nothing(t)
}

func TestAHarnessNamedAsAPathIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	site := onlyBuiltIn(t)
	spec := aJob()
	spec.Harness = "../../outside"

	code, answer := site.send(t, password, Submission{Spec: spec, Adapter: &fixture})
	if code != http.StatusBadRequest {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(answer.Refused, "outside") {
		t.Fatalf("the reason is %q", answer.Refused)
	}
	site.nothing(t)
}

// An organisation over its limit is told so, and can tell that from a fault:
// the answer is "too many" with the reason and how to lift it, and nothing is
// left on disk.
func TestAJobPastTheOrganisationsQuotaIsAnswered429WithTheReason(t *testing.T) {
	site := onlyBuiltIn(t)
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := quota.Write(path, quota.Quota{Jobs: 1, Concurrent: 1, Note: "Confirm the admin's email address to raise this limit."}); err != nil {
		t.Fatal(err)
	}
	site.index.UseQuota(quota.Open(path))

	if code, answer := site.send(t, password, Submission{Spec: aJob()}); code != http.StatusCreated {
		t.Fatalf("the first job is within the quota: %d %+v", code, answer)
	}
	code, answer := site.send(t, password, Submission{Spec: aJob()})
	if code != http.StatusTooManyRequests {
		t.Fatalf("the second job is over it: %d %+v", code, answer)
	}
	if !strings.Contains(answer.Refused, "1 task in all") || !strings.Contains(answer.Refused, "Confirm the admin's email address") {
		t.Errorf("the refusal should say what was allowed and how to lift it: %q", answer.Refused)
	}
	if len(site.index.List()) != 1 {
		t.Errorf("the refused job is in the index: %d jobs", len(site.index.List()))
	}
}
