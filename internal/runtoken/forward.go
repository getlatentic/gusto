package runtoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Forwarder is a run's end of the proxy, in its sandbox. It makes the run's key, trades the run's secret for a token bound to
// it, and passes each call of the agent's on to the proxy with the token and a proof made for that call. The agent is given the
// forwarder's address and nothing it could use anywhere else.
type Forwarder struct {
	key      ed25519.PrivateKey
	upstream *url.URL
	client   *http.Client
	now      func() time.Time
	proxy    *httputil.ReverseProxy

	mu      sync.Mutex
	token   string
	expires time.Time
}

// Place is the address an agent's configuration is written with on the host, before its forwarder has a port. gusto boxed puts
// the forwarder's address in its place.
const Place = "http://gusto-run.local"

// exchangeTimeout bounds one trade of a secret or a token, which is one small request to a proxy on the same network.
const exchangeTimeout = 15 * time.Second

// NewForwarder makes the run's key and trades secret for its first token at the proxy whose root is upstream.
func NewForwarder(upstream, secret string) (*Forwarder, error) {
	root, err := url.Parse(strings.TrimRight(upstream, "/"))
	if err != nil || root.Scheme != "http" || root.Host == "" {
		return nil, fmt.Errorf("%q is not the proxy's address", upstream)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	f := &Forwarder{key: key, upstream: root, client: &http.Client{Timeout: exchangeTimeout}, now: time.Now}
	f.proxy = &httputil.ReverseProxy{Rewrite: f.rewrite, FlushInterval: -1}
	if err := f.exchange(secret); err != nil {
		return nil, err
	}
	return f, nil
}

// exchange trades a secret, or the token held, for a new token.
func (f *Forwarder) exchange(subject string) error {
	target := f.upstream.String() + "/model/token"
	shown := ""
	if strings.Count(subject, ".") == 2 {
		shown = subject
	}
	proof, err := Prove(f.key, http.MethodPost, target, shown, f.now())
	if err != nil {
		return err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:token-exchange"}, "subject_token": {subject}}
	request, err := http.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("DPoP", proof)
	answer, err := f.client.Do(request)
	if err != nil {
		return err
	}
	defer answer.Body.Close()
	var got struct {
		Token string `json:"access_token"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(answer.Body, 16<<10)).Decode(&got); err != nil || got.Token == "" {
		return fmt.Errorf("the proxy did not give a run token: %d %s", answer.StatusCode, got.Error)
	}
	expires, err := expiryOf(got.Token)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.token, f.expires = got.Token, expires
	f.mu.Unlock()
	return nil
}

// expiryOf reads when a token the proxy gave expires. The forwarder does not check the signature: the proxy does, on every call.
func expiryOf(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("the run token cannot be read")
	}
	var claims struct {
		Expires int64 `json:"exp"`
		Issued  int64 `json:"iat"`
	}
	body, err := b64.DecodeString(parts[1])
	if err != nil || json.Unmarshal(body, &claims) != nil || claims.Expires <= claims.Issued {
		return time.Time{}, errors.New("the run token cannot be read")
	}
	return time.Unix(claims.Expires, 0), nil
}

// current is the token to show, refreshed first when less than a third of its life is left. A refresh that fails leaves the
// token held, which the proxy refuses once it has expired.
func (f *Forwarder) current() string {
	f.mu.Lock()
	token, expires := f.token, f.expires
	f.mu.Unlock()
	if left := expires.Sub(f.now()); left < tokenLife/3 {
		if f.exchange(token) == nil {
			f.mu.Lock()
			token = f.token
			f.mu.Unlock()
		}
	}
	return token
}

// tokenLife is how long a run token is good for, which a forwarder refreshes within.
const tokenLife = 15 * time.Minute

// TokenLife is how long the server makes a run token good for.
func TokenLife() time.Duration { return tokenLife }

// rewrite sends one call of the agent's to the proxy, with the token and a proof for this call, and nothing the agent sent in
// their place.
func (f *Forwarder) rewrite(out *httputil.ProxyRequest) {
	out.SetURL(f.upstream)
	out.Out.Host = f.upstream.Host
	out.Out.URL.Path, out.Out.URL.RawPath = out.In.URL.Path, out.In.URL.RawPath
	header := out.Out.Header
	header.Del("Authorization")
	header.Del("x-api-key")
	header.Del("Proxy-Authorization")
	token := f.current()
	proof, err := Prove(f.key, out.In.Method, f.upstream.Scheme+"://"+f.upstream.Host+out.In.URL.Path, token, f.now())
	if err != nil {
		return
	}
	header.Set("Authorization", "DPoP "+token)
	header.Set("DPoP", proof)
}

// ServeHTTP passes a call of the agent's on, streamed, as it answers.
func (f *Forwarder) ServeHTTP(w http.ResponseWriter, r *http.Request) { f.proxy.ServeHTTP(w, r) }
