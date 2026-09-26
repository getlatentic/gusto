package accesslog

import (
	"net/http/httptest"
	"testing"
)

func request(peer string, headers map[string]string) string {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = peer
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	return remote(r)
}

func TestBehindNginxTheClientIsInTheHeaders(t *testing.T) {
	cases := []struct {
		peer    string
		headers map[string]string
		want    string
	}{
		{"127.0.0.1:51000", map[string]string{"X-Real-IP": "198.51.100.7"}, "198.51.100.7"},
		{"[::1]:51000", map[string]string{"X-Forwarded-For": "198.51.100.7"}, "198.51.100.7"},
		// A client can start X-Forwarded-For with anything; nginx appends the
		// peer it saw, so only the last entry is believed.
		{"127.0.0.1:51000", map[string]string{"X-Forwarded-For": "10.0.0.1, 198.51.100.7"}, "198.51.100.7"},
		{"127.0.0.1:51000", map[string]string{"X-Real-IP": "198.51.100.7", "X-Forwarded-For": "10.0.0.1"}, "198.51.100.7"},
		{"127.0.0.1:51000", nil, "127.0.0.1"},
		{"127.0.0.1:51000", map[string]string{"X-Real-IP": "not an address"}, "127.0.0.1"},
	}
	for _, c := range cases {
		if got := request(c.peer, c.headers); got != c.want {
			t.Errorf("%s with %v: want %s, got %s", c.peer, c.headers, c.want, got)
		}
	}
}

// From anywhere but loopback a forwarding header is whatever the caller wrote,
// so the peer is the only address believed.
func TestFromAnywhereElseTheHeadersAreIgnored(t *testing.T) {
	lies := map[string]string{"X-Real-IP": "10.0.0.1", "X-Forwarded-For": "10.0.0.1"}
	if got := request("203.0.113.9:4000", lies); got != "203.0.113.9" {
		t.Errorf("want the peer, got %s", got)
	}
}

func TestAPeerThatIsNotAnAddressIsKeptAsGiven(t *testing.T) {
	if got := request("@", map[string]string{"X-Real-IP": "10.0.0.1"}); got != "@" {
		t.Errorf("want @, got %s", got)
	}
}
