package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Behind gusto cloud the peer is the front door on a private address, and the person's address is the one it forwards. It is not
// believed from a public peer, and a server with no front door does not believe a private peer's header.
func TestBehindTheFrontDoorTheChangeRecordNamesThePersonAndNotTheDoor(t *testing.T) {
	request := func(peer, forwarded string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = peer + ":4000"
		if forwarded != "" {
			r.Header.Set("X-Forwarded-For", forwarded)
		}
		return r
	}
	behind := behindCloud(t).server
	for name, tc := range map[string]struct {
		peer, forwarded, want string
	}{
		"the door's private address":  {"172.31.19.238", "203.0.113.7", "203.0.113.7"},
		"a chain, the last is its":    {"10.0.0.5", "198.51.100.1, 203.0.113.7", "203.0.113.7"},
		"an address that is not one":  {"172.31.19.238", "not-an-address", "172.31.19.238"},
		"nothing forwarded":           {"172.31.19.238", "", "172.31.19.238"},
		"a public peer is not a door": {"198.51.100.9", "203.0.113.7", "198.51.100.9"},
	} {
		if got := behind.clientOf(request(tc.peer, tc.forwarded)); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
	alone := open(t).server
	if got := alone.clientOf(request("172.31.19.238", "203.0.113.7")); got != "172.31.19.238" {
		t.Errorf("a server with no front door believed a private peer's header: %q", got)
	}
}
