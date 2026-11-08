package connector

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/gateway"
)

// Every connector of the catalog can be signed in to as this package does it: its server names an authorization server that takes
// PKCE, and one that needs no client registered takes a client document or registers clients itself. It asks the real apps, so it
// runs only with GUSTO_MEASURE=1.
func TestTheCatalogsAppsCanBeSignedInTo(t *testing.T) {
	if os.Getenv("GUSTO_MEASURE") != "1" {
		t.Skip("asks the real apps; set GUSTO_MEASURE=1")
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DialContext: gateway.PublicDialer(false)},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, entry := range Catalog {
		server, err := Discover(context.Background(), client, entry.URL)
		if err != nil {
			t.Errorf("%s: %v", entry.Name, err)
			continue
		}
		selfRegisters := server.ClientDocs || server.Registration != ""
		t.Logf("%-8s issuer %s, S256 %v, client document %v, registration %v", entry.Name, server.Issuer, server.ChallengeS256, server.ClientDocs, server.Registration != "")
		if !server.ChallengeS256 || selfRegisters == entry.NeedsClient {
			t.Errorf("%s does not sign in as the catalog says", entry.Name)
		}
	}
}
