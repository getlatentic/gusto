package cloud

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func cloudflareAt(server *httptest.Server) Cloudflare {
	return Cloudflare{Account: "acct123", Token: "secret-token", From: "gusto <no-reply@notifications.example.com>", API: server.URL, Wait: time.Millisecond}
}

func TestAMailGoesToCloudflareAsARequestWithTheTokenAndTheFromAddress(t *testing.T) {
	var got map[string]any
	var path, auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.Write([]byte(`{"success":true,"errors":[],"result":{"delivered":["ada@acme.com"],"permanent_bounces":[],"queued":[]}}`))
	}))
	defer server.Close()
	if err := cloudflareAt(server).Send("ada@acme.com", "Confirm your address", "Follow this link:\nhttps://x.test/verify?t=abc"); err != nil {
		t.Fatal(err)
	}
	if path != "/accounts/acct123/email/sending/send" || auth != "Bearer secret-token" {
		t.Errorf("%s %s", path, auth)
	}
	from, _ := got["from"].(map[string]any)
	if got["to"] != "ada@acme.com" || got["subject"] != "Confirm your address" || from["address"] != "no-reply@notifications.example.com" || from["name"] != "gusto" ||
		!strings.Contains(got["text"].(string), "https://x.test/verify?t=abc") {
		t.Errorf("%v", got)
	}
}

func TestCloudflareIsAskedAgainOnlyWhenAskingAgainMayWork(t *testing.T) {
	for name, tc := range map[string]struct {
		statuses []int
		want     int32
		fails    bool
	}{
		"a server error passes":     {[]int{500, 200}, 2, false},
		"a rate limit passes":       {[]int{429, 429, 200}, 3, false},
		"a server error that stays": {[]int{500, 500, 500}, 3, true},
		"a refusal is final":        {[]int{400, 200}, 1, true},
		"a wrong token is final":    {[]int{401, 200}, 1, true},
	} {
		t.Run(name, func(t *testing.T) {
			var asked atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				status := tc.statuses[min(int(asked.Add(1))-1, len(tc.statuses)-1)]
				w.WriteHeader(status)
				if status == 200 {
					w.Write([]byte(`{"success":true,"result":{"delivered":["a@b.c"],"permanent_bounces":[]}}`))
					return
				}
				w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"Sender domain not verified"}],"result":null}`))
			}))
			defer server.Close()
			err := cloudflareAt(server).Send("ada@acme.com", "hello", "body")
			if (err != nil) != tc.fails || asked.Load() != tc.want {
				t.Errorf("asked %d times, wanted %d; error %v", asked.Load(), tc.want, err)
			}
			if err != nil && (strings.Contains(err.Error(), "secret-token") || !strings.Contains(err.Error(), "Sender domain not verified")) {
				t.Errorf("the refusal says why and never the token: %v", err)
			}
		})
	}
}

func TestAnAddressCloudflareBouncedIsAFailureAndAMailWithALineBreakIsNotSent(t *testing.T) {
	var asked atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Write([]byte(`{"success":true,"errors":[],"result":{"delivered":[],"permanent_bounces":["nobody@acme.com"],"queued":[]}}`))
	}))
	defer server.Close()
	sender := cloudflareAt(server)
	if err := sender.Send("nobody@acme.com", "hello", "body"); err == nil || asked.Load() != 1 {
		t.Errorf("a bounce is reported and not repeated: %v after %d", err, asked.Load())
	}
	asked.Store(0)
	if sender.Send("ada@acme.com\r\nBcc: victim@example.org", "hello", "body") == nil || sender.Send("ada@acme.com", "hi\nBcc: x@y.z", "body") == nil || asked.Load() != 0 {
		t.Error("an address or subject with a line break is refused before anything is sent")
	}
}

func TestTheCloudflareSettingNeedsAnAccountASenderAndAToken(t *testing.T) {
	sender, err := MailFrom("cloudflare://acct123?from=gusto%20%3Cno-reply@notifications.example.com%3E", "", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := sender.(Cloudflare); !ok || got.Account != "acct123" || got.Token != "tok" || got.From != "gusto <no-reply@notifications.example.com>" {
		t.Errorf("%+v", sender)
	}
	for name, setting := range map[string]struct{ setting, token string }{
		"no token":     {"cloudflare://acct123?from=a@b.co", ""},
		"no account":   {"cloudflare://?from=a@b.co", "tok"},
		"no sender":    {"cloudflare://acct123", "tok"},
		"a bad sender": {"cloudflare://acct123?from=nonsense", "tok"},
	} {
		if _, err := MailFrom(setting.setting, "", setting.token); err == nil {
			t.Errorf("%s should be refused", name)
		}
	}
}
