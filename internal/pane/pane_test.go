package pane

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// Somebody who opens a pane late is shown the screen so far and then what is
// drawn, the same as somebody who was there from the start.
func TestALateWatcherSeesTheScreenAsItStands(t *testing.T) {
	hub := NewHub()
	pane, end := hub.Open("a-job")
	early, earlyDrawn, _ := pane.Watch()
	pane.Write([]byte("hello "))
	late, lateDrawn, _ := pane.Watch()
	pane.Write([]byte("world"))
	if len(early) != 0 || string(late) != "hello " {
		t.Fatalf("early saw %q, late saw %q", early, late)
	}
	if got := string(<-earlyDrawn) + string(<-earlyDrawn); got != "hello world" {
		t.Errorf("the early watcher saw %q", got)
	}
	if got := string(<-lateDrawn); got != "world" {
		t.Errorf("the late watcher saw %q", got)
	}
	end()
	if _, open := <-lateDrawn; open {
		t.Error("a watcher was not told the pane ended")
	}
	if _, found := hub.Get("a-job"); found {
		t.Error("an ended pane is still held")
	}
}

// What anybody types reaches the one terminal, and nothing does once it ends.
func TestWhatAnybodyTypesReachesTheTerminal(t *testing.T) {
	pane, end := NewHub().Open("a-job")
	got := make(chan []byte)
	go func() {
		read, _ := io.ReadAll(pane)
		got <- read
	}()
	if err := pane.Type([]byte("ls\r")); err != nil {
		t.Fatal(err)
	}
	end()
	select {
	case read := <-got:
		if !bytes.Equal(read, []byte("ls\r")) {
			t.Errorf("the terminal was typed %q", read)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the typing never arrived")
	}
	if err := pane.Type([]byte("x")); err == nil {
		t.Error("an ended pane took keys")
	}
}
