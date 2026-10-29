package cloud

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestABookingPageIsFramedOnlyFromAServiceThatMayBeFramed(t *testing.T) {
	for address, want := range map[string]struct {
		frame   string
		origins []string
	}{
		"https://calendly.com/tosin/gusto-demo":                           {"https://calendly.com/tosin/gusto-demo?embed_type=Inline", []string{"https://calendly.com"}},
		"https://calendar.google.com/calendar/appointments/schedules/AcZ": {"https://calendar.google.com/calendar/appointments/schedules/AcZ?gv=true", []string{"https://calendar.google.com"}},
		"https://calendar.app.google/abc123":                              {"https://calendar.app.google/abc123", []string{"https://calendar.app.google", "https://calendar.google.com"}},
		"http://calendly.com/tosin":                                       {},
		"https://calendly.com:8443/tosin":                                 {},
		"https://evil.example/calendly.com":                               {},
		"https://calendly.com.evil.example/":                              {},
		"https://cal.example/gusto":                                       {},
		"":                                                                {},
	} {
		frame, origins := bookingFrame(address)
		if frame != want.frame || !slices.Equal(origins, want.origins) {
			t.Errorf("%q: framed %q from %v, not %q from %v", address, frame, origins, want.frame, want.origins)
		}
	}
}

// Where the team gives demos and books them on a service that may be framed, the visitor picks a time on /demo itself, and the
// page lets in only that service's frames.
func TestTheBookADemoPageFramesTheBookingPage(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		set := staffSet()
		set.Demo.BookURL = "https://calendly.com/tosin/gusto-demo"
		status, page, header := newFront(t, set, nil).browser().do("GET", "/demo", nil, nil)
		if status != http.StatusOK || !strings.Contains(page, `<iframe`) || !strings.Contains(page, `src="https://calendly.com/tosin/gusto-demo?embed_type=Inline"`) || !strings.Contains(page, `href="https://calendly.com/tosin/gusto-demo"`) {
			t.Errorf("%d: the booking page is not framed\n%s", status, page)
		}
		if csp := header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src https://calendly.com") || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("the page's policy: %s", csp)
		}
		// A booking page on a service that may not be framed is linked to, and the page frames nothing.
		status, page, header = newFront(t, staffSet(), nil).browser().do("GET", "/demo", nil, nil)
		if status != http.StatusOK || strings.Contains(page, "<iframe") || !strings.Contains(page, `href="https://cal.example/gusto"`) || strings.Contains(header.Get("Content-Security-Policy"), "frame-src") {
			t.Errorf("%d: a booking page that may not be framed was framed\n%s", status, page)
		}
	})
}
