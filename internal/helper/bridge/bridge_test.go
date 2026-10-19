package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
)

// editor is a scripted client of the bridge: it says lines and reads what comes back.
type editor struct {
	t      *testing.T
	in     io.WriteCloser
	lines  chan map[string]any
	next   int
	closed chan struct{}
}

func connect(t *testing.T, agent *Agent) *editor {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	e := &editor{t: t, in: inW, lines: make(chan map[string]any, 256), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		agent.Run(ctx, inR, outW)
		outW.Close()
		close(e.closed)
	}()
	go func() {
		scanner := bufio.NewScanner(outR)
		scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
		for scanner.Scan() {
			var message map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
				t.Errorf("the bridge wrote a line that is not JSON: %q", scanner.Text())
				continue
			}
			e.lines <- message
		}
		close(e.lines)
	}()
	t.Cleanup(func() { inW.Close(); cancel() })
	return e
}

func (e *editor) write(message map[string]any) {
	e.t.Helper()
	message["jsonrpc"] = "2.0"
	text, _ := json.Marshal(message)
	if _, err := e.in.Write(append(text, '\n')); err != nil {
		e.t.Fatal(err)
	}
}

// notify sends a notification.
func (e *editor) notify(method string, params any) {
	e.write(map[string]any{"method": method, "params": params})
}

// start sends a request and gives the id to read its answer by.
func (e *editor) start(method string, params any) int {
	e.next++
	e.write(map[string]any{"id": e.next, "method": method, "params": params})
	return e.next
}

// read gives the next message, or fails when none comes.
func (e *editor) read() map[string]any {
	e.t.Helper()
	select {
	case message, open := <-e.lines:
		if !open {
			e.t.Fatal("the bridge hung up")
		}
		return message
	case <-time.After(5 * time.Second):
		e.t.Fatal("the bridge said nothing")
		return nil
	}
}

// answer reads until the answer to a request, and gives it with the session updates that came before it.
func (e *editor) answer(id int) (map[string]any, []map[string]any) {
	e.t.Helper()
	var updates []map[string]any
	for {
		message := e.read()
		if got, ok := message["id"].(float64); ok && int(got) == id {
			return message, updates
		}
		if message["method"] == "session/update" {
			updates = append(updates, message["params"].(map[string]any)["update"].(map[string]any))
		}
	}
}

func (e *editor) call(method string, params any) (map[string]any, []map[string]any) {
	e.t.Helper()
	return e.answer(e.start(method, params))
}

// cloud is a person's organization as the tests need it.
type cloud struct {
	mu        sync.Mutex
	instances []helper.Instance
	engines   []helper.Engine
	started   []string
	threads   int
	says      []string
	keys      []string
	say       func(n int) (helper.Said, error)
	follow    func(n int, told helper.Follower) (helper.Ended, error)
	stopped   []string
	instErr   error
	followed  int
}

func (c *cloud) Instances(context.Context) ([]helper.Instance, error) { return c.instances, c.instErr }
func (c *cloud) Engines(context.Context) ([]helper.Engine, error)     { return c.engines, nil }
func (c *cloud) StartInstance(_ context.Context, harness, _ string) (helper.Instance, helper.Thread, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = append(c.started, harness)
	return helper.Instance{ID: "new-" + harness, Harness: harness, Started: 1}, helper.Thread{ID: "1"}, nil
}
func (c *cloud) NewThread(_ context.Context, instance, _ string) (helper.Thread, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.threads++
	return helper.Thread{ID: fmt.Sprint(c.threads), Instance: instance}, nil
}
func (c *cloud) Say(_ context.Context, _, _, said, key string) (helper.Said, error) {
	c.mu.Lock()
	c.says, c.keys = append(c.says, said), append(c.keys, key)
	n := len(c.says)
	c.mu.Unlock()
	if c.say != nil {
		return c.say(n)
	}
	return helper.Said{Job: "job-1", State: "queued"}, nil
}
func (c *cloud) FollowJob(_ context.Context, _ string, told helper.Follower) (helper.Ended, error) {
	c.mu.Lock()
	c.followed++
	n := c.followed
	c.mu.Unlock()
	if c.follow != nil {
		return c.follow(n, told)
	}
	return passed("done"), nil
}
func (c *cloud) StopJob(_ context.Context, job string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = append(c.stopped, job)
	return nil
}

func passed(reply string) helper.Ended {
	var ended helper.Ended
	ended.Job.ID, ended.Job.State, ended.Reply = "job-1", "passed", reply
	return ended
}

func withAgents() *cloud {
	return &cloud{
		instances: []helper.Instance{{ID: "old", Harness: "opencode", Started: 10}, {ID: "newest", Title: "my agent", Harness: "claude", Started: 20}},
		engines:   []helper.Engine{{Name: "broken", Ready: false}, {Name: "opencode", Ready: true, Models: []string{"m"}}},
	}
}

func TestTheHandshakeSaysWhatTheBridgeCanDoAndWhatItCannot(t *testing.T) {
	e := connect(t, &Agent{Cloud: withAgents(), Version: "9.9"})
	answer, _ := e.call("initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}})
	result := answer["result"].(map[string]any)
	if result["protocolVersion"] != float64(1) {
		t.Errorf("version %v", result["protocolVersion"])
	}
	capabilities := result["agentCapabilities"].(map[string]any)
	if capabilities["loadSession"] != false {
		t.Errorf("the bridge says it can load a session, which it cannot yet: %v", capabilities)
	}
	if methods, ok := result["authMethods"].([]any); !ok || len(methods) != 0 {
		t.Errorf("auth methods %v", result["authMethods"])
	}
	if info := result["agentInfo"].(map[string]any); info["name"] != "gusto-helper" || info["version"] != "9.9" {
		t.Errorf("agent info %v", info)
	}
}

func TestASessionIsAThreadOnThePersonsNewestAgentOrOneStartedOrOneNamed(t *testing.T) {
	c := withAgents()
	e := connect(t, &Agent{Cloud: c})
	first, _ := e.call("session/new", map[string]any{"cwd": "/work", "mcpServers": []any{}})
	second, _ := e.call("session/new", map[string]any{"cwd": "/work", "mcpServers": []any{}})
	one, two := first["result"].(map[string]any)["sessionId"], second["result"].(map[string]any)["sessionId"]
	if one != "newest/1" || two != "newest/2" {
		t.Errorf("sessions %v %v: want threads 1 and 2 on the newest agent", one, two)
	}

	none := &cloud{engines: withAgents().engines}
	connect(t, &Agent{Cloud: none}).call("session/new", map[string]any{"cwd": "/w"})
	if len(none.started) != 1 || none.started[0] != "opencode" {
		t.Errorf("started %v: want the first engine that is ready and has models", none.started)
	}

	named := connect(t, &Agent{Cloud: withAgents(), Instance: "old"})
	if got, _ := named.call("session/new", map[string]any{"cwd": "/w"}); got["result"].(map[string]any)["sessionId"] != "old/1" {
		t.Errorf("a named agent: %v", got)
	}
	if got, _ := connect(t, &Agent{Cloud: withAgents(), Instance: "nope"}).call("session/new", map[string]any{"cwd": "/w"}); got["error"] == nil {
		t.Error("an agent the person does not have was used")
	}
}

func TestAPersonWhoIsNotSignedInIsToldToSignIn(t *testing.T) {
	c := withAgents()
	c.instErr = fmt.Errorf("%w: run `gusto-helper login` again", helper.ErrSignedOut)
	got, _ := connect(t, &Agent{Cloud: c}).call("session/new", map[string]any{"cwd": "/w"})
	failure := got["error"].(map[string]any)
	if failure["code"] != float64(-32000) || !strings.Contains(failure["message"].(string), "gusto-helper login") {
		t.Errorf("%v", failure)
	}
}

// chunks is what the agent said in a turn, joined.
func chunks(updates []map[string]any) string {
	var said string
	for _, update := range updates {
		if update["sessionUpdate"] == "agent_message_chunk" {
			said += update["content"].(map[string]any)["text"].(string)
		}
	}
	return said
}

func opened(e *editor) string {
	got, _ := e.call("session/new", map[string]any{"cwd": "/w"})
	return got["result"].(map[string]any)["sessionId"].(string)
}

func prompt(session, text string) map[string]any {
	return map[string]any{"sessionId": session, "prompt": []any{map[string]any{"type": "text", "text": text}}}
}

func TestATurnIsToldAsStepsAndOnlyTheNewWordsOfTheReplyAndEndsEndTurn(t *testing.T) {
	c := withAgents()
	c.follow = func(_ int, told helper.Follower) (helper.Ended, error) {
		told.Step(helper.Step{What: "Read", Detail: "main.go"})
		told.Reply("hel")
		told.Reply("hello")
		told.Step(helper.Step{What: "Ran", Detail: "go test"})
		told.Step(helper.Step{What: "Edited", Detail: "main.go"})
		return passed("hello world"), nil
	}
	e := connect(t, &Agent{Cloud: c})
	id := opened(e)
	got, updates := e.call("session/prompt", prompt(id, "fix it"))
	if got["result"].(map[string]any)["stopReason"] != "end_turn" {
		t.Fatalf("%v", got)
	}
	said := chunks(updates)
	if !strings.Contains(said, "in the cloud") || !strings.HasSuffix(said, "hello world") || strings.Contains(said, "hellohel") || strings.Count(said, "hello") != 1 {
		t.Errorf("the editor was told %q", said)
	}
	var calls []string
	for _, update := range updates {
		if update["sessionUpdate"] == "tool_call" {
			calls = append(calls, fmt.Sprintf("%s|%s|%s", update["kind"], update["title"], update["status"]))
		}
	}
	if strings.Join(calls, ";") != "read|Read main.go|completed;execute|Ran go test|completed;edit|Edited main.go|completed" {
		t.Errorf("tool calls: %v", calls)
	}
	if c.says[0] != "fix it" {
		t.Errorf("the cloud was told %q", c.says)
	}
	_, again := e.call("session/prompt", prompt(id, "and again"))
	if strings.Contains(chunks(again), "in the cloud") {
		t.Error("the notice about the cloud folder was said a second time")
	}
}

func TestASendThatFailedIsTriedAgainWithTheSameKeyAndIsOneTurn(t *testing.T) {
	c := withAgents()
	c.say = func(n int) (helper.Said, error) {
		if n < 3 {
			return helper.Said{}, errors.New("connection reset")
		}
		return helper.Said{Job: "job-1"}, nil
	}
	e := connect(t, &Agent{Cloud: c})
	got, _ := e.call("session/prompt", prompt(opened(e), "do it"))
	if got["result"] == nil {
		t.Fatalf("%v", got)
	}
	if len(c.keys) != 3 || c.keys[0] != c.keys[1] || c.keys[1] != c.keys[2] || !strings.HasPrefix(c.keys[0], "acp-") {
		t.Errorf("the sends carried keys %v: they have to be one key", c.keys)
	}
}

func TestAFollowThatDropsIsMadeAgainAndNothingIsToldTwice(t *testing.T) {
	c := withAgents()
	c.follow = func(n int, told helper.Follower) (helper.Ended, error) {
		told.Step(helper.Step{What: "Read", Detail: "a.go"})
		told.Reply("one")
		if n == 1 {
			return helper.Ended{}, io.ErrUnexpectedEOF
		}
		told.Step(helper.Step{What: "Ran", Detail: "make"})
		return passed("one two"), nil
	}
	e := connect(t, &Agent{Cloud: c})
	_, updates := e.call("session/prompt", prompt(opened(e), "go"))
	reads, runs := 0, 0
	for _, update := range updates {
		if update["sessionUpdate"] == "tool_call" {
			if strings.HasPrefix(update["title"].(string), "Read") {
				reads++
			} else {
				runs++
			}
		}
	}
	if reads != 1 || runs != 1 || !strings.HasSuffix(chunks(updates), "one two") || strings.Count(chunks(updates), "one") != 1 {
		t.Errorf("reads %d runs %d, said %q", reads, runs, chunks(updates))
	}
}

func TestACancelStopsTheTurnInTheCloudAndTheTurnEndsCancelled(t *testing.T) {
	c := withAgents()
	release := make(chan struct{})
	c.follow = func(_ int, told helper.Follower) (helper.Ended, error) {
		told.Step(helper.Step{What: "Ran", Detail: "sleep"})
		<-release
		var ended helper.Ended
		ended.Job.ID, ended.Job.State, ended.Job.StoppedBy = "job-1", "interrupted", "ada@acme.example"
		return ended, nil
	}
	e := connect(t, &Agent{Cloud: c})
	id := opened(e)
	asked := e.start("session/prompt", prompt(id, "a long one"))
	for { // wait until the turn is under way
		if m := e.read(); m["method"] == "session/update" && m["params"].(map[string]any)["update"].(map[string]any)["sessionUpdate"] == "tool_call" {
			break
		}
	}
	e.notify("session/cancel", map[string]any{"sessionId": id})
	deadline := time.Now().Add(3 * time.Second)
	for {
		c.mu.Lock()
		n := len(c.stopped)
		c.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(c.stopped) != 1 || c.stopped[0] != "job-1" {
		t.Fatalf("the cloud was asked to stop %v", c.stopped)
	}
	close(release)
	got, _ := e.answer(asked)
	if got["result"].(map[string]any)["stopReason"] != "cancelled" {
		t.Errorf("the turn ended %v", got)
	}
}

func TestACancelThatComesBeforeTheTurnIsSentStopsItAsItIsSent(t *testing.T) {
	c := withAgents()
	gate := make(chan struct{})
	c.say = func(int) (helper.Said, error) { <-gate; return helper.Said{Job: "job-9"}, nil }
	c.follow = func(int, helper.Follower) (helper.Ended, error) {
		var ended helper.Ended
		ended.Job.State, ended.Job.StoppedBy = "interrupted", "me"
		return ended, nil
	}
	e := connect(t, &Agent{Cloud: c})
	id := opened(e)
	asked := e.start("session/prompt", prompt(id, "hurry"))
	time.Sleep(100 * time.Millisecond)
	e.notify("session/cancel", map[string]any{"sessionId": id})
	time.Sleep(100 * time.Millisecond)
	close(gate)
	got, _ := e.answer(asked)
	c.mu.Lock()
	stopped := append([]string(nil), c.stopped...)
	c.mu.Unlock()
	if got["result"].(map[string]any)["stopReason"] != "cancelled" || len(stopped) != 1 || stopped[0] != "job-9" {
		t.Errorf("%v, stopped %v", got, stopped)
	}
}

func TestAFailedTurnSaysWhyInWordsAndASecondPromptWhileBusyIsRefused(t *testing.T) {
	c := withAgents()
	release := make(chan struct{})
	c.follow = func(n int, told helper.Follower) (helper.Ended, error) {
		<-release
		var ended helper.Ended
		ended.Job.State, ended.Stopped = "failed", "the model refused the call"
		return ended, nil
	}
	e := connect(t, &Agent{Cloud: c})
	id := opened(e)
	first := e.start("session/prompt", prompt(id, "one"))
	time.Sleep(100 * time.Millisecond)
	busy, _ := e.call("session/prompt", prompt(id, "two"))
	if busy["error"] == nil || busy["error"].(map[string]any)["code"] != float64(-32600) {
		t.Errorf("a second prompt while busy gave %v", busy)
	}
	close(release)
	got, updates := e.answer(first)
	if got["result"].(map[string]any)["stopReason"] != "end_turn" || !strings.Contains(chunks(updates), "The turn failed: the model refused the call") {
		t.Errorf("%v %q", got, chunks(updates))
	}
}

func TestAReplyThatDoesNotCarryOnFromWhatWasToldIsNotToldAgain(t *testing.T) {
	c := withAgents()
	c.follow = func(_ int, told helper.Follower) (helper.Ended, error) {
		told.Reply("the start of a long answer")
		return passed("…the end of a long answer, cut at the front"), nil
	}
	e := connect(t, &Agent{Cloud: c})
	_, updates := e.call("session/prompt", prompt(opened(e), "x"))
	if strings.Contains(chunks(updates), "the end of a long answer") {
		t.Errorf("a cut reply was told as if it carried on: %q", chunks(updates))
	}
}

func TestWhatTheBridgeDoesNotKnowIsAnsweredOrIgnoredAndBadJSONIsAParseError(t *testing.T) {
	e := connect(t, &Agent{Cloud: withAgents()})
	if got, _ := e.call("fs/read_text_file", map[string]any{}); got["error"].(map[string]any)["code"] != float64(-32601) {
		t.Errorf("%v", got)
	}
	e.notify("$/something", map[string]any{})
	if got, _ := e.call("session/prompt", prompt("no/such", "x")); got["error"] == nil {
		t.Error("a prompt for a session that does not exist was accepted")
	}
	if got, _ := e.call("session/prompt", map[string]any{"sessionId": opened(e), "prompt": []any{}}); got["error"] == nil {
		t.Error("a prompt with nothing in it was accepted")
	}
	if _, err := io.WriteString(e.in, "this is not json\n"); err != nil {
		t.Fatal(err)
	}
	if got := e.read(); got["error"].(map[string]any)["code"] != float64(-32700) {
		t.Errorf("%v", got)
	}
}

func TestTheEditorHangingUpEndsTheBridgeAndLeavesTheTurnAlone(t *testing.T) {
	c := withAgents()
	e := connect(t, &Agent{Cloud: c})
	opened(e)
	e.in.Close()
	select {
	case <-e.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("the bridge did not end when the editor hung up")
	}
	if len(c.stopped) != 0 {
		t.Errorf("hanging up stopped %v: closing an editor does not stop a turn", c.stopped)
	}
}
