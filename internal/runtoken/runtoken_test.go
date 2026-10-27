package runtoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const exchangeURL = "http://host.openshell.internal:8041/model/token"
const callURL = "http://host.openshell.internal:8041/model/v1/chat/completions"

func anIssuer(t *testing.T) (*Issuer, *time.Time) {
	t.Helper()
	issuer, err := NewIssuer(10*time.Minute, func(run string) Run { return Run{Person: "ada@acme.com", Agent: "a1", Room: "room-launch"} })
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	issuer.now = func() time.Time { return now }
	return issuer, &now
}

func aKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// secrets is a run's one bootstrap secret, which redeem takes once.
func secrets() func(string) (string, bool) {
	left := map[string]string{"boot": "run-1"}
	return func(secret string) (string, bool) {
		run, ok := left[secret]
		delete(left, secret)
		return run, ok
	}
}

func exchange(t *testing.T, issuer *Issuer, key ed25519.PrivateKey, subject string, redeem func(string) (string, bool)) (string, error) {
	t.Helper()
	shown := ""
	if strings.Count(subject, ".") == 2 {
		shown = subject
	}
	proof, err := Prove(key, "POST", exchangeURL, shown, issuer.now())
	if err != nil {
		t.Fatal(err)
	}
	return issuer.Exchange(subject, proof, "POST", exchangeURL, redeem)
}

func call(t *testing.T, issuer *Issuer, key ed25519.PrivateKey, token string) (string, error) {
	t.Helper()
	proof, err := Prove(key, "POST", callURL, token, issuer.now())
	if err != nil {
		t.Fatal(err)
	}
	return issuer.Check(token, proof, "POST", callURL)
}

func TestARunsSecretIsTradedOnceForATokenBoundToTheSandboxsKey(t *testing.T) {
	issuer, _ := anIssuer(t)
	key, redeem := aKey(t), secrets()
	token, err := exchange(t, issuer, key, "boot", redeem)
	if err != nil {
		t.Fatal(err)
	}
	if run, err := call(t, issuer, key, token); err != nil || run != "run-1" {
		t.Fatalf("a call with the token and its key gave %q, %v", run, err)
	}
	if _, err := exchange(t, issuer, aKey(t), "boot", redeem); err == nil {
		t.Error("the secret was traded twice")
	}
	if _, err := call(t, issuer, aKey(t), token); err == nil {
		t.Error("the token worked with a key it is not bound to")
	}
}

func TestATokenSaysWhoAndWhatItIsForAsRFC9068AndRFC9396NameIt(t *testing.T) {
	issuer, _ := anIssuer(t)
	token, err := exchange(t, issuer, aKey(t), "boot", secrets())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := b64.DecodeString(strings.Split(token, ".")[1])
	var claims map[string]any
	json.Unmarshal(body, &claims)
	details, _ := json.Marshal(claims["authorization_details"])
	if claims["sub"] != "ada@acme.com" || claims["client_id"] != "a1" || claims["aud"] != Audience || claims["run"] != "run-1" ||
		claims["cnf"].(map[string]any)["jkt"] == "" || string(details) != `[{"actions":["model","tools"],"room":"room-launch","type":"gusto_run"}]` {
		t.Errorf("the claims are %v", claims)
	}
}

func TestAProofIsGoodForOneRequestOnce(t *testing.T) {
	issuer, _ := anIssuer(t)
	key := aKey(t)
	token, _ := exchange(t, issuer, key, "boot", secrets())
	proof, _ := Prove(key, "POST", callURL, token, issuer.now())
	if _, err := issuer.Check(token, proof, "POST", callURL); err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Check(token, proof, "POST", callURL); err == nil {
		t.Error("a proof was accepted twice")
	}
	other, _ := Prove(key, "POST", callURL, token, issuer.now())
	if _, err := issuer.Check(token, other, "GET", callURL); err == nil {
		t.Error("a proof for one method was accepted for another")
	}
	forAnother, _ := Prove(key, "POST", callURL, "another-token", issuer.now())
	if _, err := issuer.Check(token, forAnother, "POST", callURL); err == nil {
		t.Error("a proof for another token was accepted")
	}
}

func TestATokenExpiresAndARefreshBeforeThenKeepsTheSameKey(t *testing.T) {
	issuer, now := anIssuer(t)
	key := aKey(t)
	token, _ := exchange(t, issuer, key, "boot", secrets())
	*now = now.Add(9 * time.Minute)
	fresh, err := exchange(t, issuer, key, token, nil)
	if err != nil {
		t.Fatalf("a token still good was not refreshed: %v", err)
	}
	*now = now.Add(2 * time.Minute)
	if _, err := call(t, issuer, key, token); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("an expired token gave %v", err)
	}
	if _, err := call(t, issuer, key, fresh); err != nil {
		t.Errorf("the refreshed token gave %v", err)
	}
	if _, err := exchange(t, issuer, aKey(t), fresh, nil); err == nil {
		t.Error("a token was refreshed by a key it is not bound to")
	}
}

func TestAStaleProofOrAnotherServersTokenIsRefused(t *testing.T) {
	issuer, _ := anIssuer(t)
	key := aKey(t)
	token, _ := exchange(t, issuer, key, "boot", secrets())
	stale, _ := Prove(key, "POST", callURL, token, issuer.now().Add(-2*time.Minute))
	if _, err := issuer.Check(token, stale, "POST", callURL); err == nil {
		t.Error("a stale proof was accepted")
	}
	other, _ := anIssuer(t)
	if _, err := call(t, other, key, token); err == nil {
		t.Error("another server's token was accepted")
	}
}

func TestReplaysAreRememberedUpToABoundAndThenRefused(t *testing.T) {
	r := &replays{seen: map[string]time.Time{}, most: 2}
	now := time.Unix(1_800_000_000, 0)
	if !r.first("a", now) || !r.first("b", now) || r.first("c", now) {
		t.Error("a third fresh proof was taken past the bound")
	}
	if !r.first("c", now.Add(3*proofWindow)) {
		t.Error("old proofs were not forgotten to make room")
	}
}
