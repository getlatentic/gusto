package web

import (
	"regexp"
	"testing"
)

// The conversation's live regions are positioned out of sight. Their nearest positioned ancestor has to be the conversation's
// own scroller, or they sit below the whole thread, the page grows past the window, and jumping to a new turn scrolls the app
// up and leaves a blank band over the demo's bar.
func TestTheConversationsLiveRegionsStayInsideItsScroller(t *testing.T) {
	work := &roomWork{}
	site, _, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	page := site.as(t, alice).ask(t, "GET", thread, nil).Body.String()
	scroller := regexp.MustCompile(`<div class="([^"]*)" id="said"`).FindStringSubmatch(page)
	if scroller == nil || !regexp.MustCompile(`(^| )relative( |$)`).MatchString(scroller[1]) {
		t.Fatalf("the conversation's scroller is not positioned: %v", scroller)
	}
}
