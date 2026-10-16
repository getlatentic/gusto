package web

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/registry"
)

func (s *site) postAs(secret, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+secret)
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestAPersonStartsAnAgentByTheAPIUnderTheSameRulesAsThePage(t *testing.T) {
	here := withTokens(t)
	ada, ben := signInADevice(t, here, alice), signInADevice(t, here, bob)

	started := here.postAs(ada, "/api/instances", `{"harness":"fixture","title":"my agent"}`)
	if started.Code != http.StatusCreated || !strings.Contains(started.Body.String(), `"who":"`+alice+`"`) || !strings.Contains(started.Body.String(), `"thread"`) {
		t.Fatalf("starting an agent gave %d %s", started.Code, started.Body)
	}
	if list := here.getAs(ada, "/api/instances").Body.String(); !strings.Contains(list, "my agent") {
		t.Errorf("alice's list lacks her agent: %s", list)
	}
	if list := here.getAs(ben, "/api/instances").Body.String(); strings.Contains(list, "my agent") {
		t.Errorf("bob was shown alice's agent: %s", list)
	}
	for name, body := range map[string]string{
		"an engine nobody has":      `{"harness":"nonesuch"}`,
		"a repository on the disk":  `{"harness":"fixture","repo":"/etc"}`,
		"something that is no JSON": `harness=fixture`,
	} {
		if got := here.postAs(ada, "/api/instances", body); got.Code != http.StatusBadRequest {
			t.Errorf("%s gave %d %s", name, got.Code, got.Body)
		}
	}
}

func TestALimitedPersonsTokenIsOfferedAndMayStartOnlyTheirModels(t *testing.T) {
	here, _, _ := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big"}})
	admin.ask(t, "POST", "/admin/models/providers/acme/enable", nil)
	admin.ask(t, "POST", "/admin/models/providers/acme/models/enable", url.Values{"model": {"big"}})
	admin.ask(t, "POST", "/admin/models/access", url.Values{"email": {alice}, "model": {"acme/big"}})
	ada, ben := signInADevice(t, here, alice), signInADevice(t, here, bob)

	engines := here.getAs(ada, "/api/engines").Body.String()
	if !strings.Contains(engines, "acme/big") || strings.Contains(engines, "platform/qwen") {
		t.Errorf("a limited person's token was offered %s", engines)
	}
	if bobs := here.getAs(ben, "/api/engines").Body.String(); !strings.Contains(bobs, "platform/qwen") || !strings.Contains(bobs, "acme/big") {
		t.Errorf("an unlimited person was offered %s", bobs)
	}
	if got := here.postAs(ada, "/api/instances", `{"harness":"fixture","model":"platform/qwen"}`); got.Code != http.StatusBadRequest {
		t.Errorf("a limited person's token started a model they may not use: %d %s", got.Code, got.Body)
	}
	if got := here.postAs(ada, "/api/instances", `{"harness":"fixture","model":"acme/big"}`); got.Code != http.StatusCreated {
		t.Errorf("the model they are limited to was refused: %d %s", got.Code, got.Body)
	}
}

// turnDone waits for the test queue to finish a job.
func turnDone(t *testing.T, here *tokened, id string) jobs.Record {
	t.Helper()
	var record jobs.Record
	waitUntil(t, "the job to finish", func() bool {
		found, ok := here.index.Get(id)
		record = found
		return ok && found.State.Done()
	})
	return record
}

func waitUntil(t *testing.T, what string, ready func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not happen", what)
}

func TestATurnIsReadAsJSONWithItsStepsItsReplyAndWhatChanged(t *testing.T) {
	here := withTokens(t)
	_, thread := started(t, here.as(t, alice))
	ada := signInADevice(t, here, alice)
	_, said := here.say("/api"+thread+"/say", ada, "read the file", "k1")
	id := said["job"].(string)
	record := turnDone(t, here, id)
	os.WriteFile(record.Dir(here.index.Root()).OutFile("commands.jsonl"), []byte(`{"tool":"read","status":"completed","input":{"path":"main.go"}}`+"\n"), 0o644)

	got := here.getAs(ada, "/api/jobs/"+id)
	var turn struct {
		Job   struct{ State, Brief string }
		Steps []struct{ What, Detail string }
		Reply string
	}
	if got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &turn) != nil {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	if turn.Job.State != "passed" || turn.Job.Brief != "read the file" || !strings.Contains(turn.Reply, "first line") || len(turn.Steps) != 1 || turn.Steps[0].What != "Read" || !strings.Contains(turn.Steps[0].Detail, "main.go") {
		t.Errorf("the turn read as %+v", turn)
	}
	if got := here.getAs(signInADevice(t, here, bob), "/api/jobs/"+id); got.Code != http.StatusNotFound {
		t.Errorf("bob read alice's turn: %d", got.Code)
	}
	if got := here.getAs(here.secrets["everyone"], "/api/jobs/"+id); got.Code == http.StatusOK {
		t.Errorf("an organisation's token read a person's turn")
	}
}

// follows is a person's connection to one turn's stream.
func follow(t *testing.T, here *tokened, secret, id string) *listening {
	t.Helper()
	server := httptest.NewServer(here.handler)
	t.Cleanup(server.Close)
	request, _ := http.NewRequest("GET", server.URL+"/api/jobs/"+id+"/follow", nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	heard := &listening{t: t, frames: make(chan string, 64), body: response}
	go func() {
		var frame []string
		lines := bufio.NewScanner(response.Body)
		for lines.Scan() {
			if lines.Text() == "" {
				if len(frame) > 0 {
					heard.frames <- strings.Join(frame, "\n")
				}
				frame = nil
				continue
			}
			frame = append(frame, lines.Text())
		}
		close(heard.frames)
	}()
	return heard
}

func TestARunningTurnIsFollowedStepByStepToItsEnd(t *testing.T) {
	was := followEvery
	followEvery = 20 * time.Millisecond
	t.Cleanup(func() { followEvery = was })
	here := withTokens(t)
	ada := signInADevice(t, here, alice)
	id := addTurnJob(t, here, jobs.Record{State: jobs.Running, Brief: "work", Harness: "fixture", Asked: alice, Queued: time.Now().Unix(), Started: time.Now().Unix()})
	dir := here.index.Root() + "/" + id
	os.MkdirAll(dir+"/out", 0o755)
	os.WriteFile(dir+"/out/transcript.log", []byte("thinking"), 0o644)

	heard := follow(t, here, ada, id)
	if reply := heard.next(); !strings.Contains(reply, "event: reply") || !strings.Contains(reply, `"text":"thinking"`) {
		t.Fatalf("first %q", reply)
	}
	os.WriteFile(dir+"/out/commands.jsonl", []byte(`{"tool":"bash","input":{"command":"go test"}}`+"\n"), 0o644)
	if step := heard.next(); !strings.Contains(step, "event: step") || !strings.Contains(step, `"what":"Ran"`) {
		t.Fatalf("step %q", step)
	}
	os.WriteFile(dir+"/out/transcript.log", []byte("all done"), 0o644)
	if err := here.index.Finish(id, jobs.Passed, 0); err != nil {
		t.Fatal(err)
	}
	var ended string
	for ended == "" {
		frame := heard.next()
		if strings.Contains(frame, "event: ended") {
			ended = frame
		}
	}
	if !strings.Contains(ended, `"state":"passed"`) || !strings.Contains(ended, "all done") {
		t.Errorf("the end was %q", ended)
	}
	if a := follow(t, here, signInADevice(t, here, bob), id); a == nil {
		t.Fatal("no stream")
	}
}

func TestSomebodyElsesTurnCannotBeFollowed(t *testing.T) {
	here := withTokens(t)
	id := addTurnJob(t, here, jobs.Record{State: jobs.Passed, Brief: "mine", Harness: "fixture", Asked: alice, Queued: time.Now().Unix(), Ended: time.Now().Unix()})
	if got := here.getAs(signInADevice(t, here, bob), "/api/jobs/"+id+"/follow"); got.Code != http.StatusNotFound {
		t.Errorf("bob followed alice's turn: %d", got.Code)
	}
	if got := here.getAs(signInADevice(t, here, alice), "/api/jobs/"+id+"/follow"); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "event: ended") {
		t.Errorf("a finished turn was followed as %d %s", got.Code, got.Body)
	}
}
