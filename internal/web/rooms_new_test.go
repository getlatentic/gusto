package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestARefusedRoomNameIsSaidOnThePageThatMakesRooms(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	mine := site.as(t, alice)
	refused := mine.ask(t, "POST", "/rooms", url.Values{"name": {"   "}})
	where := refused.Header().Get("Location")
	if refused.Code != http.StatusSeeOther || !strings.HasPrefix(where, "/rooms/new?refused=") {
		t.Fatalf("an empty name gave %d to %q", refused.Code, where)
	}
	var page struct{ Refused string }
	mine.data(t, where, &page)
	if page.Refused == "" {
		t.Error("the page that makes rooms does not say why the name was refused")
	}
	var fresh struct{ Refused string }
	mine.data(t, "/rooms/new", &fresh)
	if fresh.Refused != "" {
		t.Errorf("a fresh page says %q", fresh.Refused)
	}
}
