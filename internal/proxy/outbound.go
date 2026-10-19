package proxy

import (
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/spend"
)

// outbound answers a container using the proxy as an ordinary forward proxy,
// through HTTPS_PROXY and HTTP_PROXY: a CONNECT for HTTPS, or a request with a
// whole URL for plain HTTP. Nothing leaves unless the allowlist names the host,
// and every request, sent or refused, is a line in the job's proxy record.
func (p *Proxy) outbound(w http.ResponseWriter, r *http.Request) {
	ticket, known := p.tokens.Job(proxyToken(r))
	if !known {
		w.Header().Set("Proxy-Authenticate", `Basic realm="gusto"`)
		http.Error(w, "no", http.StatusProxyAuthRequired)
		return
	}
	address := destination(r)
	sent := time.Now()
	call := spend.Call{Path: r.URL.Path, Host: address, AtUnix: sent.Unix()}
	if !p.allowed.Permits(address) {
		call.Status, call.Denied = http.StatusForbidden, "not on the allowlist"
		p.note(ticket.Job, call)
		http.Error(w, "gusto refused "+address+": it is not on this job's allowlist", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodConnect {
		p.tunnel(w, ticket.Job, address, call)
		return
	}
	p.note(ticket.Job, p.forward(w, r, call))
}

// forward sends a plain HTTP request on as it came, less the proxy's own
// headers. No credential is added: the proxy holds one for the model only.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, call spend.Call) spend.Call {
	out := r.Clone(r.Context())
	out.RequestURI = ""
	for _, name := range []string{"Proxy-Authorization", "Proxy-Connection", "Connection"} {
		out.Header.Del(name)
	}
	answer, err := p.outboundTransport().RoundTrip(out)
	if err != nil {
		call.Error = err.Error()
		http.Error(w, "that host did not answer", http.StatusBadGateway)
		return call
	}
	defer answer.Body.Close()
	copyHeaders(w.Header(), answer.Header)
	w.WriteHeader(answer.StatusCode)
	call.Bytes, _ = io.Copy(relay{to: w}, answer.Body)
	call.Status = answer.StatusCode
	return call
}

// destination is host:port, with the port a scheme implies when none is given.
func destination(r *http.Request) string {
	host := r.URL.Host
	if r.Method == http.MethodConnect || host == "" {
		host = r.Host
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	if r.URL.Scheme == "https" || r.Method == http.MethodConnect {
		return net.JoinHostPort(host, "443")
	}
	return net.JoinHostPort(host, "80")
}

// proxyToken is the job's token as a forward-proxy client presents it: the
// password in a proxy URL such as http://gusto:<token>@127.0.0.1:8041, which a
// client sends as Proxy-Authorization.
func proxyToken(r *http.Request) string {
	given := r.Header.Get("Proxy-Authorization")
	if encoded, found := strings.CutPrefix(given, "Basic "); found {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return ""
		}
		_, password, _ := strings.Cut(string(decoded), ":")
		return password
	}
	bearer, _ := strings.CutPrefix(given, "Bearer ")
	return strings.TrimSpace(bearer)
}

// outboundTransport carries a job's plain HTTP requests, connecting through dial
// so the same rules hold as for a tunnel, and following no redirect: the
// container asks for each address itself.
func (p *Proxy) outboundTransport() *http.Transport {
	return &http.Transport{DialContext: p.dial, DisableKeepAlives: true, Proxy: nil}
}
