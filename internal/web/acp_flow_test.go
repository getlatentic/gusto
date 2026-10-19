package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
	"github.com/getlatentic/gusto/internal/helper/bridge"
	"github.com/getlatentic/gusto/internal/jobs"
)

// anEditor is an editor speaking the protocol to the bridge, which is talking to the real server.
type anEditor struct {
	t    *testing.T
	in   io.WriteCloser
	read chan map[string]any
	id   int
}

func editorOnTheRealServer(t *testing.T, here *tokened, person string) *anEditor {
	t.Helper()
	server := httptest.NewServer(here.handler)
	t.Cleanup(server.Close)
	agent := &bridge.Agent{Cloud: helper.Client{Server: server.URL, Token: signInADevice(t, here, person)}, Version: "test"}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { agent.Run(ctx, inR, outW); outW.Close() }()
	e := &anEditor{t: t, in: inW, read: make(chan map[string]any, 256)}
	go func() {
		lines := bufio.NewScanner(outR)
		lines.Buffer(make([]byte, 0, 64<<10), 8<<20)
		for lines.Scan() {
			var message map[string]any
			if json.Unmarshal(lines.Bytes(), &message) == nil {
				e.read <- message
			}
		}
		close(e.read)
	}()
	t.Cleanup(func() { inW.Close(); cancel() })
	return e
}

func (e *anEditor) send(message map[string]any) {
	message["jsonrpc"] = "2.0"
	text, _ := json.Marshal(message)
	e.in.Write(append(text, '\n'))
}

func (e *anEditor) start(method string, params any) int {
	e.id++
	e.send(map[string]any{"id": e.id, "method": method, "params": params})
	return e.id
}

func (e *anEditor) next() map[string]any {
	e.t.Helper()
	select {
	case message, open := <-e.read:
		if !open {
			e.t.Fatal("the bridge hung up")
		}
		return message
	case <-time.After(10 * time.Second):
		e.t.Fatal("nothing came from the bridge")
		return nil
	}
}

func (e *anEditor) answer(id int) (map[string]any, string) {
	var said strings.Builder
	for {
		message := e.next()
		if got, ok := message["id"].(float64); ok && int(got) == id {
			return message, said.String()
		}
		if message["method"] == "session/update" {
			update := message["params"].(map[string]any)["update"].(map[string]any)
			if update["sessionUpdate"] == "agent_message_chunk" {
				said.WriteString(update["content"].(map[string]any)["text"].(string))
			}
		}
	}
}

func (e *anEditor) call(method string, params any) (map[string]any, string) {
	return e.answer(e.start(method, params))
}

func TestAnEditorTalksToTheRealServerThroughTheBridgeAndHearsTheAnswer(t *testing.T) {
	here := withTokens(t)
	e := editorOnTheRealServer(t, here, alice)
	if init, _ := e.call("initialize", map[string]any{"protocolVersion": 1}); init["result"] == nil {
		t.Fatalf("%v", init)
	}
	opened, _ := e.call("session/new", map[string]any{"cwd": "/work"})
	session := opened["result"].(map[string]any)["sessionId"].(string)
	got, said := e.call("session/prompt", map[string]any{"sessionId": session, "prompt": []any{map[string]any{"type": "text", "text": "say something"}}})
	if got["result"].(map[string]any)["stopReason"] != "end_turn" {
		t.Fatalf("%v", got)
	}
	if !strings.Contains(said, "first line") || !strings.Contains(said, "second line") {
		t.Errorf("the editor was told %q, and the agent's reply is the transcript's two lines", said)
	}
	mine := 0
	for _, record := range here.index.List() {
		if record.Asked == alice && strings.HasPrefix(record.Key, "acp-") {
			mine++
		}
	}
	if mine != 1 {
		t.Errorf("%d turns carry the bridge's key, and the one prompt made one", mine)
	}
}

func TestAnEditorCancelsATurnOnTheRealServerAndItReallyStopsAndIsTold(t *testing.T) {
	started := holding(t)
	here := withTokens(t)
	e := editorOnTheRealServer(t, here, alice)
	e.call("initialize", map[string]any{"protocolVersion": 1})
	opened, _ := e.call("session/new", map[string]any{"cwd": "/work"})
	session := opened["result"].(map[string]any)["sessionId"].(string)
	asked := e.start("session/prompt", map[string]any{"sessionId": session, "prompt": []any{map[string]any{"type": "text", "text": "a long piece of work"}}})
	<-started
	e.send(map[string]any{"method": "session/cancel", "params": map[string]any{"sessionId": session}})
	got, _ := e.answer(asked)
	if got["result"].(map[string]any)["stopReason"] != "cancelled" {
		t.Fatalf("the turn ended %v", got)
	}
	var found bool
	for _, record := range here.index.List() {
		if record.State == jobs.Interrupted && record.StoppedBy == alice {
			found = true
		}
	}
	if !found {
		t.Errorf("the turn in the cloud was not stopped by alice: %+v", here.index.List())
	}
}
