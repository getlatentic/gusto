package cloud

import (
	"fmt"
	"time"
)

// Step is one stage of getting a demo ready, and Mark says whether it is done, under way or still to come.
type Step struct {
	Label string
	Mark  string
}

var demoStages = []string{"Getting a computer ready", "Setting up your demo", "Opening your demo"}

// demoSteps is where a demo is among its stages, from the state its server is in.
func demoSteps(state State) []Step {
	at := 0
	switch state {
	case Starting:
		at = 1
	case Ready:
		at = len(demoStages)
	}
	steps := make([]Step, len(demoStages))
	for i, label := range demoStages {
		steps[i] = Step{Label: label, Mark: "todo"}
		switch {
		case i < at:
			steps[i].Mark = "done"
		case i == at:
			steps[i].Mark = "now"
		}
	}
	return steps
}

// waited says how long a visitor has waited, to the second for the first minutes.
func waited(since time.Time, now time.Time) string {
	if since.IsZero() || now.Before(since) {
		return ""
	}
	seconds := int(now.Sub(since).Round(time.Second) / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%d s", seconds)
	}
	return fmt.Sprintf("%d min %02d s", seconds/60, seconds%60)
}

// orgExists says an organization is still there, whether or not its server is running.
func (s *Server) orgExists(slug string) bool {
	_, found, _ := s.Store.Org(slug)
	return found
}
