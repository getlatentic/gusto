package web

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/helper"
)

// The helper on a person's computer signs in to the real server, is told who it is, and signs out, which ends its token.
func TestTheHelperSignsInAsAPersonAndSignsOutAgainstTheRealServer(t *testing.T) {
	here := withTokens(t)
	server := httptest.NewServer(here.handler)
	defer server.Close()

	token, person, err := helper.Login{
		Server: server.URL, Label: "Ada's laptop", Wait: func(context.Context, time.Duration) error { return nil },
		Show: func(address, code string) {
			if !strings.Contains(address, "/device?code="+code) {
				t.Errorf("the address %q does not carry the code %q", address, code)
			}
			if got := here.as(t, alice).ask(t, "POST", "/device", url.Values{"code": {code}}); !strings.Contains(got.Body.String(), "Approved") {
				t.Errorf("the person could not approve:\n%s", got.Body)
			}
		},
	}.Run(context.Background())
	if err != nil || person != alice || token == "" {
		t.Fatalf("%q %q %v", token, person, err)
	}
	client := helper.Client{Server: server.URL, Token: token}
	if who, err := client.Me(context.Background()); err != nil || who.Person != alice {
		t.Fatalf("%+v %v", who, err)
	}
	if err := client.End(context.Background()); err != nil {
		t.Fatalf("signing out: %v", err)
	}
	if _, err := client.Me(context.Background()); err == nil || !strings.Contains(err.Error(), "gusto login") {
		t.Errorf("a token that signed out still worked: %v", err)
	}
	if ended := ofKind(written(t, here.log), changelog.TokenRevoked); len(ended) != 1 || ended[0].Who != alice {
		t.Errorf("signing out was not recorded against alice: %+v", ended)
	}
}
