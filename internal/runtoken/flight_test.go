package runtoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestManyCallsAsATokenNearsItsEndRefreshItOnce(t *testing.T) {
	var trades atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trades.Add(1)
		time.Sleep(50 * time.Millisecond)
		now := time.Now().Unix()
		claims, _ := json.Marshal(map[string]int64{"iat": now, "exp": now + 900})
		json.NewEncoder(w).Encode(map[string]string{"access_token": "h." + b64.EncodeToString(claims) + ".s"})
	}))
	defer proxy.Close()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	root, _ := url.Parse(proxy.URL)
	f := &Forwarder{prove: KeyProver(key), upstream: root, client: proxy.Client(), now: time.Now,
		token: "h.old.s", expires: time.Now().Add(time.Minute)}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); f.current() }()
	}
	wg.Wait()
	if got := trades.Load(); got != 1 {
		t.Errorf("sixteen calls near the token's end traded it %d times", got)
	}
}

func TestARunThatHasEndedIsNotGivenANewToken(t *testing.T) {
	issuer, err := NewIssuer(TokenLife(), func(string) Run { return Run{} })
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	token, err := exchange(t, issuer, key, "boot", secrets())
	if err != nil {
		t.Fatal(err)
	}
	proof, err := Prove(key, "POST", exchangeURL, token, issuer.now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Exchange(token, proof, "POST", exchangeURL, secrets(), func(string) bool { return false }); err == nil {
		t.Error("a run that has ended was given a new token")
	}
}
