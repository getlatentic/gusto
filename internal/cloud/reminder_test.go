package cloud

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// An organization's pages remind a person who has not confirmed their address, and mail the link again, by asking
// the front door from the organization's own address. No other address is answered.
func TestAnOrganizationsPagesAskWhetherThePersonHasConfirmedAndMailTheLinkAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServers(t)
		ada := f.signedUp("ada@acme.com")
		from := map[string]string{"Origin": "http://acme-com.cloud.test"}

		confirmed := func(b *browser) confirmation {
			t.Helper()
			status, body, header := b.do("GET", "/account.json", nil, from)
			var answer confirmation
			if err := json.Unmarshal([]byte(body), &answer); status != http.StatusOK || err != nil || header.Get("Access-Control-Allow-Origin") != from["Origin"] {
				t.Fatalf("%d %v %s %v", status, err, body, header)
			}
			return answer
		}
		if got := confirmed(ada); got != (confirmation{Email: "ada@acme.com", Jobs: DefaultLimits().Verified.Jobs}) {
			t.Errorf("a new admin has not confirmed: %+v", got)
		}

		before := len(f.mails())
		if status, _, header := ada.do("POST", "/account/confirm", nil, from); status != http.StatusNoContent || header.Get("Access-Control-Allow-Origin") != from["Origin"] {
			t.Fatalf("the organization's page asks for the link again: %d %v", status, header)
		}
		mails := f.mails()
		if len(mails) != before+1 || !strings.Contains(mails[len(mails)-1], "To: ada@acme.com") || !strings.Contains(mails[len(mails)-1], "/verify?t=") {
			t.Fatalf("the link is mailed again: %d mails", len(mails)-before)
		}

		for _, origin := range []string{"http://acme-com--0123456789.cloud.test", "https://acme-com.cloud.test", "http://evil.example"} {
			other := map[string]string{"Origin": origin}
			if status, _, header := ada.do("GET", "/account.json", nil, other); status != http.StatusForbidden || header.Get("Access-Control-Allow-Origin") != "" {
				t.Errorf("%s is told nothing: %d %v", origin, status, header)
			}
			if status, _, _ := ada.do("POST", "/account/confirm", nil, other); status != http.StatusForbidden {
				t.Errorf("%s may not have the link mailed: %d", origin, status)
			}
		}
		if status, _, _ := ada.do("GET", "/account.json", nil, map[string]string{"Origin": "http://globex-com.cloud.test"}); status != http.StatusUnauthorized {
			t.Errorf("an organization she is not in is told nothing of her: %d", status)
		}
		if status, _, _ := f.browser().do("GET", "/account.json", nil, from); status != http.StatusUnauthorized {
			t.Errorf("somebody signed out is told nothing: %d", status)
		}

		link := f.lastLink()
		f.browser().post("/verify", url.Values{"t": {link[strings.Index(link, "t=")+2:]}})
		if got := confirmed(ada); !got.Confirmed {
			t.Errorf("once the link is followed, there is nothing to remind her of: %+v", got)
		}
		before = len(f.mails())
		if status, _, _ := ada.do("POST", "/account/confirm", nil, from); status != http.StatusNoContent || len(f.mails()) != before {
			t.Errorf("a confirmed address is not mailed: %d", status)
		}
	})
}
