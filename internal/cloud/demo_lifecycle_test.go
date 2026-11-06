package cloud

import (
	"errors"
	"testing"
)

// A demo's time starts when its server first answers, whoever opens it, so one a colleague opened by the organization's own
// address is not ended as never ready. spec/Demo.before-clock.cfg is the trace.
func TestADemosTimeStartsWhenItsServerAnswers(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s, _ := clocked(t, demoSet)
		c := controlled(t, s)
		_, org, err := s.StartDemo(t.Context(), "Ada", "ada@acme.com", "Acme")
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		demo, _, _ := s.DemoLeft(t.Context(), org.Slug)
		if demo.Ready.IsZero() {
			t.Fatal("a demo whose server answers has not started its time")
		}
		if due, _ := s.DemosDue(t.Context()); due[org.Slug] != "" {
			t.Errorf("a demo in use is due: %s", due[org.Slug])
		}
	})
}

// A demo the visitor stopped is not extended, and its time does not start. spec/Demo.before-guards.cfg is the trace.
func TestAStoppedDemoIsNotExtendedOrStarted(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s, _ := clocked(t, demoSet)
		_, org, err := s.StartDemo(t.Context(), "Ada", "ada@acme.com", "Acme")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.StopDemo(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		if demo, _ := s.DemoReady(t.Context(), org.Slug); !demo.Ready.IsZero() {
			t.Error("a stopped demo's time started")
		}
		_, other, _ := s.StartDemo(t.Context(), "Bob", "bob@globex.com", "Globex")
		s.DemoReady(t.Context(), other.Slug)
		s.StopDemo(t.Context(), other.Slug)
		if _, err := s.ExtendDemo(t.Context(), other.Slug); !errors.Is(err, ErrNoExtension) {
			t.Errorf("a stopped demo was extended: %v", err)
		}
	})
}

// A demo the sweep has begun to end is not provisioned again, by a front door that starts partway through the end or by
// anything else. spec/Demo.before-ending.cfg is the trace.
func TestADemoBeingEndedIsNotMadeAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s, _ := clocked(t, demoSet)
		c := controlled(t, s)
		_, org, err := s.StartDemo(t.Context(), "Ada", "ada@acme.com", "Acme")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkEnding(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		if err := c.Provision(t.Context(), org.Slug); !errors.Is(err, ErrRemoved) {
			t.Errorf("a demo being ended was provisioned: %v", err)
		}
		if c.Node.Alive(org.Slug) {
			t.Error("a demo being ended has a server")
		}
		if due, _ := s.DemosDue(t.Context()); due[org.Slug] == "" {
			t.Error("a demo whose ending was cut off is not due")
		}
	})
}
