package desk

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/helper"
)

func turn(job, state string) helper.Turn {
	return helper.Turn{Job: job, Instance: "i1", Thread: "t1", Brief: "write the report", State: state}
}

func TestMenuListsRunningTurnsBeforeEnded(t *testing.T) {
	status := helper.Status{SignedIn: true, Person: "ada", Connected: true}
	lines := Menu("https://haas.example.com/", status, []helper.Turn{turn("a", "passed"), turn("b", "running"), turn("c", "queued")})
	var labels []string
	for _, line := range lines {
		labels = append(labels, line.Label)
	}
	want := []string{"Signed in as ada", "write the report — working", "write the report — waiting", "write the report — finished"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("menu = %q, want %q", labels, want)
	}
	if lines[0].Page != "" || lines[1].Page != "https://haas.example.com/instances/i1/threads/t1" {
		t.Fatalf("pages = %q, %q", lines[0].Page, lines[1].Page)
	}
}

func TestMenuStandingSaysWhatIsWrong(t *testing.T) {
	for status, want := range map[helper.Status]string{
		{}:                                "Not signed in: run haas-helper login",
		{SignedIn: true, Person: "ada"}:   "Reconnecting…",
		{SignedIn: true, Connected: true}: "Signed in as ",
	} {
		if got := Menu("s", status, nil)[0].Label; got != want {
			t.Errorf("%+v: %q, want %q", status, got, want)
		}
	}
}

func TestMenuHoldsAtMostItsSlots(t *testing.T) {
	var turns []helper.Turn
	for i := range 30 {
		turns = append(turns, turn(string(rune('a'+i)), "passed"))
	}
	if got := len(Menu("s", helper.Status{}, turns)); got != Slots+1 {
		t.Fatalf("lines = %d, want %d", got, Slots+1)
	}
}

func TestPageOfAJobWithNoConversationIsTheJob(t *testing.T) {
	if got := PageOf("https://s", helper.Turn{Job: "j9"}); got != "https://s/jobs/j9" {
		t.Fatal(got)
	}
}

func TestShortKeepsOneLineThatFits(t *testing.T) {
	got := short("  first\n\nsecond   " + strings.Repeat("x", 100))
	if strings.ContainsAny(got, "\n") || len([]rune(got)) != briefWidth || !strings.HasSuffix(got, "…") {
		t.Fatalf("%q", got)
	}
	if short("  ") != "a turn" {
		t.Fatal("an empty brief has a name")
	}
}

func hear(t *testing.T, hub *helper.Hub, before []helper.Turn, then ...helper.Turn) []Note {
	t.Helper()
	for _, turn := range before {
		hub.Tell(helper.Event{Kind: "turn", Turn: turn})
	}
	notes := make(chan Note, 8)
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	announcer := &Announcer{Hub: hub, Say: func(n Note) { notes <- n }}
	go func() { announcer.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	for _, turn := range then {
		hub.Tell(helper.Event{Kind: "turn", Turn: turn})
	}
	time.Sleep(100 * time.Millisecond)
	stop()
	<-done
	close(notes)
	var out []Note
	for note := range notes {
		out = append(out, note)
	}
	return out
}

func TestAnnouncerNamesATurnItSawGoing(t *testing.T) {
	notes := hear(t, helper.NewHub(), []helper.Turn{turn("a", "running")}, turn("a", "passed"))
	if len(notes) != 1 || notes[0].Title != "Your agent finished" || notes[0].Body != "write the report" {
		t.Fatalf("%+v", notes)
	}
}

func TestAnnouncerSaysFailedAndStopped(t *testing.T) {
	notes := hear(t, helper.NewHub(), []helper.Turn{turn("a", "queued"), turn("b", "running")}, turn("a", "failed"), turn("b", "interrupted"))
	if len(notes) != 2 || notes[0].Title != "Your agent failed" || notes[1].Title != "Your agent stopped" {
		t.Fatalf("%+v", notes)
	}
}

func TestAnnouncerIsSilentForTurnsTheServerRepeats(t *testing.T) {
	notes := hear(t, helper.NewHub(), []helper.Turn{turn("a", "passed")}, turn("a", "passed"), turn("z", "passed"))
	if len(notes) != 0 {
		t.Fatalf("%+v", notes)
	}
}

func TestAnnouncerSaysATurnOnceWhenItEnds(t *testing.T) {
	notes := hear(t, helper.NewHub(), []helper.Turn{turn("a", "running")}, turn("a", "passed"), turn("a", "passed"))
	if len(notes) != 1 {
		t.Fatalf("%+v", notes)
	}
}

func TestNotifyKeepsTheTextOutOfTheScript(t *testing.T) {
	hostile := Note{Title: `"; do shell script "x" --`, Body: "it's <b>$(id)</b>\n"}
	mac := notifyCommand("darwin", hostile)
	for _, arg := range mac.Args[:len(mac.Args)-2] {
		if strings.Contains(arg, "shell script") || strings.Contains(arg, "$(id)") {
			t.Fatalf("text in the script: %q", arg)
		}
	}
	windows := notifyCommand("windows", hostile)
	for _, arg := range windows.Args {
		if strings.Contains(arg, "$(id)") {
			t.Fatalf("text in the script: %q", arg)
		}
	}
	if !strings.Contains(strings.Join(windows.Env, "\x00"), "HAAS_NOTE_BODY=it's <b>$(id)</b>\n") {
		t.Fatal("the text is not in the environment")
	}
	if linux := notifyCommand("linux", hostile); linux.Args[len(linux.Args)-3] != "--" {
		t.Fatalf("a title starting with a dash would be an option: %q", linux.Args)
	}
}
