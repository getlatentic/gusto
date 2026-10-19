package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/jobs"
)

// say sends something into a thread with a person's token and, when it has one, a request id.
func (s *site) say(path, secret, said, key string) (int, map[string]any) {
	request := httptest.NewRequest("POST", path, strings.NewReader(`{"said":`+quote(said)+`}`))
	request.Header.Set("Authorization", "Bearer "+secret)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	var out map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &out)
	return recorder.Code, out
}

func quote(text string) string { out, _ := json.Marshal(text); return string(out) }

func countJobsAskedBy(s *site, who string) (n int) {
	for _, record := range s.index.List() {
		if record.Asked == who {
			n++
		}
	}
	return n
}

func TestARetriedSendIsTheSameTurnAndAnotherIdIsAnother(t *testing.T) {
	here := withTokens(t)
	_, thread := started(t, here.as(t, alice))
	secret := signInADevice(t, here, alice)
	path := "/api" + thread + "/say"

	code, first := here.say(path, secret, "add a test", "send-1")
	if code != http.StatusAccepted || first["already"] != false || first["job"] == "" {
		t.Fatalf("the first send gave %d %+v", code, first)
	}
	code, again := here.say(path, secret, "add a test", "send-1")
	if code != http.StatusAccepted || again["already"] != true || again["job"] != first["job"] {
		t.Errorf("the retry gave %d %+v, not the first job again", code, again)
	}
	if n := countJobsAskedBy(here.site, alice); n != 1 {
		t.Errorf("a retried send made %d jobs", n)
	}
	if _, other := here.say(path, secret, "add a test", "send-2"); other["job"] == first["job"] || other["already"] != false {
		t.Errorf("another id was taken for the first: %+v", other)
	}
	if _, bare := here.say(path, secret, "add a test", ""); bare["already"] != false || countJobsAskedBy(here.site, alice) != 3 {
		t.Errorf("sends with no id were joined: %+v", bare)
	}
}

func TestAnIdBelongsToThePersonWhoSentItAndLastsAsLongAsTheJob(t *testing.T) {
	here := withTokens(t)
	_, hers := started(t, here.as(t, alice))
	_, his := started(t, here.as(t, bob))
	ada, ben := signInADevice(t, here, alice), signInADevice(t, here, bob)

	_, a := here.say("/api"+hers+"/say", ada, "hello", "same-words")
	_, b := here.say("/api"+his+"/say", ben, "hello", "same-words")
	if a["job"] == b["job"] || b["already"] != false {
		t.Errorf("bob's id was taken for alice's: %+v %+v", a, b)
	}
	reopened, err := jobs.Open(here.index.Root())
	if err != nil {
		t.Fatal(err)
	}
	if found, ok := reopened.FindKey(alice, "same-words"); !ok || found.ID != a["job"] {
		t.Errorf("after a restart the id was lost: %+v %v", found, ok)
	}
	if _, ok := reopened.FindKey(bob, "no-such-id"); ok {
		t.Error("an id nobody sent was found")
	}
}

func TestSendsThatArriveTogetherWithOneIdAreOneTurn(t *testing.T) {
	here := withTokens(t)
	_, thread := started(t, here.as(t, alice))
	secret := signInADevice(t, here, alice)
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			here.say("/api"+thread+"/say", secret, "all at once", "burst")
		}()
	}
	wait.Wait()
	if n := countJobsAskedBy(here.site, alice); n != 1 {
		t.Errorf("eight sends with one id made %d jobs", n)
	}
}

func TestABadIdOrNothingSaidOrSomebodyElsesThreadIsRefused(t *testing.T) {
	here := withTokens(t)
	_, thread := started(t, here.as(t, alice))
	secret := signInADevice(t, here, alice)
	path := "/api" + thread + "/say"
	if code, _ := here.say(path, secret, "hi", strings.Repeat("k", 129)); code != http.StatusBadRequest {
		t.Errorf("a long id gave %d", code)
	}
	if code, _ := here.say(path, secret, "hi", "has a\x01control"); code != http.StatusBadRequest {
		t.Errorf("an unprintable id gave %d", code)
	}
	if code, _ := here.say(path, secret, "   ", "k"); code != http.StatusBadRequest {
		t.Errorf("saying nothing gave %d", code)
	}
	if code, _ := here.say(path, signInADevice(t, here, bob), "hi", "k"); code != http.StatusNotFound {
		t.Errorf("bob saying something in alice's thread gave %d", code)
	}
	if n := countJobsAskedBy(here.site, alice) + countJobsAskedBy(here.site, bob); n != 0 {
		t.Errorf("a refused send made %d jobs", n)
	}
}

func TestAPersonOpensAThreadOfTheirOwnAndNotSomebodyElses(t *testing.T) {
	here := withTokens(t)
	where, _ := started(t, here.as(t, alice))
	id := strings.TrimPrefix(where, "/instances/")
	request := func(secret string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/instances/"+id+"/threads", strings.NewReader(`{"title":"a second thing"}`))
		r.Header.Set("Authorization", "Bearer "+secret)
		w := httptest.NewRecorder()
		here.handler.ServeHTTP(w, r)
		return w
	}
	if got := request(signInADevice(t, here, alice)); got.Code != http.StatusCreated || !strings.Contains(got.Body.String(), `"title":"a second thing"`) {
		t.Errorf("alice's new thread gave %d %s", got.Code, got.Body)
	}
	if got := request(signInADevice(t, here, bob)); got.Code != http.StatusNotFound {
		t.Errorf("bob opened a thread on alice's agent: %d", got.Code)
	}
}

// post sends a job with an organisation's token and a request id.
func (s *tokened) postJob(secret, key string) (int, Answer) {
	body, _ := json.Marshal(Submission{Spec: aJob()})
	request := httptest.NewRequest("POST", "/api/jobs", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+secret)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	var answer Answer
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	return recorder.Code, answer
}

func TestAJobSentTwiceWithOneIdIsOneJobForTheSameTokenAndTwoForAnother(t *testing.T) {
	here := withTokens(t)
	code, first := here.postJob(here.secrets["ci"], "build-42")
	if code != http.StatusCreated || first.ID == "" {
		t.Fatalf("%d %+v", code, first)
	}
	if code, again := here.postJob(here.secrets["ci"], "build-42"); code != http.StatusOK || again.ID != first.ID {
		t.Errorf("the retry gave %d %+v, not the first job again", code, again)
	}
	if _, other := here.postJob(here.secrets["everyone"], "build-42"); other.ID == first.ID || other.ID == "" {
		t.Errorf("another token's job with the same id was joined: %+v", other)
	}
	if n := len(here.index.List()); n != 2 {
		t.Errorf("%d jobs were made", n)
	}
	if code, _ := here.postJob(here.secrets["ci"], strings.Repeat("x", 200)); code != http.StatusBadRequest {
		t.Errorf("a long id gave %d", code)
	}
}
