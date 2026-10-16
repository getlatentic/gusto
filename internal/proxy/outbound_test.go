package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/spend"
)

// gateway is the proxy served on a real listener, as a container reaches it,
// with one job's token minted and the given hosts allowed.
type gateway struct {
	server *httptest.Server
	token  string
	record string
	ledger *spend.Ledger
}

func openGateway(t *testing.T, allowed string) gateway {
	t.Helper()
	tokens := NewTokens()
	kept := t.TempDir()
	ledger := spend.NewLedger(spend.Policy{})
	proxy := New(tokens, Credentials{"": {Address: "http://127.0.0.1:1"}},
		func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true },
		ledger, ParseAllowlist(allowed))
	server := httptest.NewServer(proxy.Handler())
	t.Cleanup(server.Close)
	token, err := tokens.Mint("a-job", "")
	if err != nil {
		t.Fatal(err)
	}
	return gateway{server: server, token: token, record: filepath.Join(kept, "a-job.jsonl"), ledger: ledger}
}

// client is what a harness in the box uses: HTTPS_PROXY with the job's token
// as the password.
func (g gateway) client(t *testing.T, token string, trusted *httptest.Server) *http.Client {
	t.Helper()
	through, err := url.Parse(g.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	through.User = url.UserPassword("gusto", token)
	transport := &http.Transport{Proxy: http.ProxyURL(through)}
	if trusted != nil {
		transport.TLSClientConfig = trusted.Client().Transport.(*http.Transport).TLSClientConfig
	}
	return &http.Client{Transport: transport}
}

// reached is a host that counts the requests it was sent.
func reached(t *testing.T, tls bool) (*httptest.Server, *int) {
	t.Helper()
	count := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Write([]byte("the host answered"))
	})
	server := httptest.NewUnstartedServer(handler)
	if tls {
		server.StartTLS()
	} else {
		server.Start()
	}
	t.Cleanup(server.Close)
	return server, &count
}

func hostOf(server *httptest.Server) string {
	return strings.TrimPrefix(strings.TrimPrefix(server.URL, "https://"), "http://")
}

func TestAHostOnTheAllowlistIsReachedOverHTTPS(t *testing.T) {
	host, count := reached(t, true)
	gateway := openGateway(t, hostOf(host))
	answer, err := gateway.client(t, gateway.token, host).Get(host.URL + "/package")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(answer.Body)
	answer.Body.Close()
	if string(body) != "the host answered" || *count != 1 {
		t.Fatalf("answer %q, the host saw %d", body, *count)
	}
	calls, _ := spend.ReadCalls(gateway.record)
	if len(calls) != 1 || calls[0].Host != hostOf(host) || calls[0].Status != 200 {
		t.Fatalf("the record is %+v", calls)
	}
}

// Refused and recorded: the line in out/proxy.jsonl is what puts the attempt
// in the job's evidence, because the manifest hashes that file.
func TestAHostNotOnTheAllowlistIsRefusedAndRecorded(t *testing.T) {
	for _, tls := range []bool{true, false} {
		host, count := reached(t, tls)
		gateway := openGateway(t, "registry.npmjs.org")
		answer, err := gateway.client(t, gateway.token, host).Get(host.URL + "/exfiltrate")
		if err == nil {
			answer.Body.Close()
			if answer.StatusCode != http.StatusForbidden {
				t.Fatalf("tls %t: code %d", tls, answer.StatusCode)
			}
		}
		if *count != 0 {
			t.Fatalf("tls %t: a host nobody allowed was sent %d requests", tls, *count)
		}
		calls, _ := spend.ReadCalls(gateway.record)
		if len(calls) != 1 || calls[0].Denied == "" || calls[0].Host != hostOf(host) || calls[0].Status != 403 {
			t.Fatalf("tls %t: the record is %+v", tls, calls)
		}
		if report := gateway.ledger.Allow("a-job", "", ""); report != nil {
			t.Fatalf("a refused host was counted as spend: %+v", report)
		}
	}
}

func TestAPlainHTTPHostOnTheAllowlistIsReached(t *testing.T) {
	host, count := reached(t, false)
	gateway := openGateway(t, hostOf(host))
	answer, err := gateway.client(t, gateway.token, nil).Get(host.URL + "/simple")
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	if answer.StatusCode != 200 || *count != 1 {
		t.Fatalf("code %d, the host saw %d", answer.StatusCode, *count)
	}
}

func TestTheOutboundWayNeedsAJobsToken(t *testing.T) {
	host, count := reached(t, false)
	gateway := openGateway(t, hostOf(host))
	answer, err := gateway.client(t, "not-a-token", nil).Get(host.URL)
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	if answer.StatusCode != http.StatusProxyAuthRequired || *count != 0 {
		t.Fatalf("code %d, the host saw %d", answer.StatusCode, *count)
	}
}

// A sealed container reaches the proxy through a socket, not a port.
func TestTheProxyAnswersOnASocket(t *testing.T) {
	tokens := NewTokens()
	token, _ := tokens.Mint("a-job", "")
	host, count := reached(t, false)
	proxy := New(tokens, Credentials{"": {Address: "http://127.0.0.1:1"}},
		func(string) (string, bool) { return "", false }, spend.NewLedger(spend.Policy{}), ParseAllowlist(hostOf(host)))
	socket := filepath.Join(t.TempDir(), "p.sock")
	stop, err := proxy.ListenSocket(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	transport := &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "box", User: url.UserPassword("gusto", token)}),
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}
	answer, err := (&http.Client{Transport: transport}).Get(host.URL)
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	if answer.StatusCode != 200 || *count != 1 {
		t.Fatalf("code %d, the host saw %d", answer.StatusCode, *count)
	}
}
