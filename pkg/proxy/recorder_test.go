package proxy

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/pkg/spend"
)

type noted struct {
	task string
	line int
	call spend.Call
}

type hearing struct {
	mu   sync.Mutex
	seen []noted
}

func (l *hearing) Record(task string, line int, call spend.Call) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, noted{task, line, call})
}

type failing struct{}

func (failing) Record(string, int, spend.Call) { panic("the index is down") }

func askOnce(t *testing.T, answering *model, p *Proxy, token string) {
	t.Helper()
	ask(t, p, "/model/"+token+"/v1/chat/completions", `{"model":"fixture-model"}`)
}

func TestTheRecorderIsToldEachCallWithTheLineItWasWrittenAt(t *testing.T) {
	answering := &model{contentType: "application/json", answer: plainAnswer}
	proxy, token, record, _ := counted(t, answering, spend.Terms{})
	heard := &hearing{}
	proxy.UseRecorder(heard)
	for range 3 {
		askOnce(t, answering, proxy, token)
	}
	if len(heard.seen) != 3 {
		t.Fatalf("the recorder heard %d calls", len(heard.seen))
	}
	for i, each := range heard.seen {
		if each.task != "a-job" || each.line != i || each.call.Model != "fixture-model" {
			t.Fatalf("call %d was told as %+v", i, each)
		}
	}
	if got := len(calls(t, record)); got != 3 {
		t.Fatalf("the file holds %d calls", got)
	}
}

func TestLinesAreCountedFromWhatTheFileAlreadyHolds(t *testing.T) {
	answering := &model{contentType: "application/json", answer: plainAnswer}
	proxy, token, record, _ := counted(t, answering, spend.Terms{})
	if err := os.WriteFile(record, []byte("{\"path\":\"/a\"}\n{\"path\":\"/b\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	heard := &hearing{}
	proxy.UseRecorder(heard)
	askOnce(t, answering, proxy, token)
	askOnce(t, answering, proxy, token)
	if len(heard.seen) != 2 || heard.seen[0].line != 2 || heard.seen[1].line != 3 {
		t.Fatalf("the recorder heard %+v", heard.seen)
	}
}

func TestTheRecordIsWrittenWithNoRecorderAndWhenTheRecorderFails(t *testing.T) {
	for name, recorder := range map[string]Recorder{"none": nil, "one that panics": failing{}} {
		t.Run(name, func(t *testing.T) {
			answering := &model{contentType: "application/json", answer: plainAnswer}
			proxy, token, record, _ := counted(t, answering, spend.Terms{})
			proxy.UseRecorder(recorder)
			askOnce(t, answering, proxy, token)
			askOnce(t, answering, proxy, token)
			text, err := os.ReadFile(record)
			if err != nil || strings.Count(string(text), "\n") != 2 {
				t.Fatalf("the file is %q (%v)", text, err)
			}
		})
	}
}

func TestACallThatCouldNotBeWrittenIsNotToldToTheRecorder(t *testing.T) {
	answering := &model{contentType: "application/json", answer: plainAnswer}
	proxy, token, record, _ := counted(t, answering, spend.Terms{})
	heard := &hearing{}
	proxy.UseRecorder(heard)
	if err := os.Mkdir(record, 0o755); err != nil {
		t.Fatal(err)
	}
	askOnce(t, answering, proxy, token)
	if len(heard.seen) != 0 {
		t.Fatalf("the recorder heard %+v", heard.seen)
	}
}
