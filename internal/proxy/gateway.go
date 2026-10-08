package proxy

import (
	"net/http"
	"strings"
)

// Gateways gives the handler that is a job's gateway to its tool servers, or says the job has none.
type Gateways func(job string) (http.Handler, bool)

// UseGateways lets a job reach its tool servers through the proxy, at /mcp. The job's token says whose they are, as it says whose
// credential a model call uses, and what a job may reach is what its setup was given and not what its harness asks for.
func (p *Proxy) UseGateways(gateways Gateways) { p.gateways.Store(&gateways) }

// toolsPath is where a job's gateway answers, after the token is taken off.
func toolsPath(rest string) bool { return rest == "/mcp" || strings.HasPrefix(rest, "/mcp/") }

func (p *Proxy) tools(w http.ResponseWriter, r *http.Request, ticket Ticket) {
	held := p.gateways.Load()
	if held == nil {
		http.NotFound(w, r)
		return
	}
	handler, has := (*held)(ticket.Job)
	if !has {
		http.NotFound(w, r)
		return
	}
	handler.ServeHTTP(w, r)
}
