package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// zoneServer is enough of Cloudflare's DNS API to keep records in memory and count what was done.
type zoneServer struct {
	sync.Mutex
	records map[string]record
	calls   []string
	next    int
	zones   int
}

func (s *zoneServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Lock()
	defer s.Unlock()
	answer := func(status int, result any, problem string) {
		w.WriteHeader(status)
		if problem != "" {
			json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []map[string]any{{"code": 9109, "message": problem}}, "result": nil})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": result})
	}
	if r.Header.Get("Authorization") != "Bearer good-token" {
		answer(403, nil, "Invalid access token")
		return
	}
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	switch {
	case r.URL.Path == "/zones":
		found := []map[string]string{}
		for i := 0; i < s.zones; i++ {
			found = append(found, map[string]string{"id": "zone1"})
		}
		answer(200, found, "")
	case r.URL.Path == "/zones/zone1/dns_records" && r.Method == "GET":
		found := []record{}
		for _, each := range s.records {
			if each.Name == r.URL.Query().Get("name") {
				found = append(found, each)
			}
		}
		answer(200, found, "")
	case r.URL.Path == "/zones/zone1/dns_records" && r.Method == "POST":
		var made record
		json.NewDecoder(r.Body).Decode(&made)
		s.next++
		made.ID = fmt.Sprint("r", s.next)
		s.records[made.ID] = made
		answer(200, made, "")
	case strings.HasPrefix(r.URL.Path, "/zones/zone1/dns_records/"):
		id := strings.TrimPrefix(r.URL.Path, "/zones/zone1/dns_records/")
		if r.Method == "DELETE" {
			delete(s.records, id)
			answer(200, map[string]string{"id": id}, "")
			return
		}
		var changed record
		json.NewDecoder(r.Body).Decode(&changed)
		changed.ID = id
		s.records[id] = changed
		answer(200, changed, "")
	default:
		answer(404, nil, "no such route")
	}
}

func setUp(t *testing.T) (*zoneServer, string, string) {
	t.Helper()
	fake := &zoneServer{records: map[string]record{}, zones: 1}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	token := filepath.Join(t.TempDir(), "token")
	os.WriteFile(token, []byte("good-token\n"), 0o600)
	return fake, server.URL, token
}

func namesAndIPs(s *zoneServer) map[string][]string {
	found := map[string][]string{}
	for _, each := range s.records {
		found[each.Name] = append(found[each.Name], each.Content)
	}
	return found
}

func TestTheNameAndEveryNameUnderItArePointedAtTheHostAndOnlyChangedWhenTheyDiffer(t *testing.T) {
	fake, api, token := setUp(t)
	if err := run(api, "example.com", "gusto.example.com", "203.0.113.7", token, false); err != nil {
		t.Fatal(err)
	}
	got := namesAndIPs(fake)
	if len(got) != 2 || got["gusto.example.com"][0] != "203.0.113.7" || got["*.gusto.example.com"][0] != "203.0.113.7" {
		t.Fatalf("%v", got)
	}
	for _, each := range fake.records {
		if each.Proxied || each.TTL != 60 || each.Type != "A" {
			t.Errorf("a name that is served from the host itself is not proxied, and lasts a minute: %+v", each)
		}
	}
	fake.calls = nil
	if err := run(api, "example.com", "gusto.example.com", "203.0.113.7", token, false); err != nil {
		t.Fatal(err)
	}
	for _, call := range fake.calls {
		if strings.HasPrefix(call, "POST") || strings.HasPrefix(call, "PUT") || strings.HasPrefix(call, "DELETE") {
			t.Errorf("a record that is right was written again: %v", fake.calls)
		}
	}
	if err := run(api, "example.com", "gusto.example.com", "198.51.100.9", token, false); err != nil {
		t.Fatal(err)
	}
	got = namesAndIPs(fake)
	if len(got) != 2 || len(got["gusto.example.com"]) != 1 || got["gusto.example.com"][0] != "198.51.100.9" || got["*.gusto.example.com"][0] != "198.51.100.9" {
		t.Errorf("a new host takes the names over and leaves one record for each: %v", got)
	}
}

func TestExtraRecordsForOneNameAreReducedToOne(t *testing.T) {
	fake, api, token := setUp(t)
	fake.records["a"] = record{ID: "a", Type: "A", Name: "gusto.example.com", Content: "192.0.2.1", TTL: 300}
	fake.records["b"] = record{ID: "b", Type: "A", Name: "gusto.example.com", Content: "192.0.2.2", TTL: 300}
	if err := run(api, "example.com", "gusto.example.com", "203.0.113.7", token, false); err != nil {
		t.Fatal(err)
	}
	if got := namesAndIPs(fake)["gusto.example.com"]; len(got) != 1 || got[0] != "203.0.113.7" {
		t.Errorf("%v", got)
	}
}

func TestRemovingTakesTheNameAndEveryNameUnderItAway(t *testing.T) {
	fake, api, token := setUp(t)
	fake.records["x"] = record{ID: "x", Type: "A", Name: "other.example.com", Content: "192.0.2.1"}
	run(api, "example.com", "gusto.example.com", "203.0.113.7", token, false)
	if err := run(api, "example.com", "gusto.example.com", "", token, true); err != nil {
		t.Fatal(err)
	}
	if got := namesAndIPs(fake); len(got) != 1 || len(got["other.example.com"]) != 1 {
		t.Errorf("only this host's names go: %v", got)
	}
	if err := run(api, "example.com", "gusto.example.com", "", token, true); err != nil {
		t.Errorf("names that are already gone are done: %v", err)
	}
}

func TestWhatIsWrongIsRefusedByNameAndTheTokenIsNeverSaid(t *testing.T) {
	fake, api, token := setUp(t)
	bad := filepath.Join(t.TempDir(), "bad")
	os.WriteFile(bad, []byte("wrong-token"), 0o600)
	if err := run(api, "example.com", "gusto.example.com", "203.0.113.7", bad, false); err == nil || !strings.Contains(err.Error(), "Invalid access token") || strings.Contains(err.Error(), "wrong-token") {
		t.Errorf("%v", err)
	}
	fake.zones = 0
	if err := run(api, "example.com", "gusto.example.com", "203.0.113.7", token, false); err == nil || !strings.Contains(err.Error(), "0 zones") {
		t.Errorf("a token that sees no zone says so: %v", err)
	}
	fake.zones = 1
	for name, args := range map[string][3]string{
		"a name outside the zone": {"example.com", "gusto.example.org", "203.0.113.7"},
		"a lookalike zone suffix": {"example.com", "gustoexample.com", "203.0.113.7"},
		"an IPv6 address":         {"example.com", "gusto.example.com", "2001:db8::1"},
		"no address":              {"example.com", "gusto.example.com", ""},
		"no name":                 {"example.com", "", "203.0.113.7"},
	} {
		if err := run(api, args[0], args[1], args[2], token, false); err == nil {
			t.Errorf("%s should be refused", name)
		}
	}
}
