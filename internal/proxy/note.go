package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/getlatentic/haas/internal/spend"
)

// note writes a call to the job's out/proxy.jsonl and counts it in the ledger.
// The line is the ledger: a server that restarts rebuilds its counts from these
// lines, and the manifest hashes them with the rest of the evidence.
func (p *Proxy) note(job string, call spend.Call) {
	p.counts.add(call)
	p.ledger.Count(job, call)
	where, ok := p.where(job)
	if !ok {
		return
	}
	line, err := json.Marshal(call)
	if err != nil {
		return
	}
	p.guard.Lock()
	defer p.guard.Unlock()
	file, err := os.OpenFile(where, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Error("the proxy could not write down a call", "job", job, "error", err)
		return
	}
	defer file.Close()
	if _, err := fmt.Fprintf(file, "%s\n", line); err != nil {
		slog.Error("the proxy could not write down a call", "job", job, "error", err)
	}
}

// refuse answers a call a cap stopped, in the error shape an OpenAI-compatible
// client already prints, so the reason reaches the harness's own transcript.
// 402 rather than 429, because a client retries a 429 and a spent cap does not
// come back by waiting.
func (p *Proxy) refuse(w http.ResponseWriter, job string, call spend.Call) {
	p.note(job, call)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"message": "haas stopped this job: " + call.Refused.Reason,
			"type":    "haas_cap",
			"code":    call.Refused.Cap + "_cap",
		},
	})
}

// relay hands the container each piece of an answer as it arrives. A streamed
// answer held in a buffer until it fills is a harness waiting on a model that
// already spoke.
type relay struct{ to http.ResponseWriter }

func (r relay) Write(given []byte) (int, error) {
	written, err := r.to.Write(given)
	if flusher, ok := r.to.(http.Flusher); ok {
		flusher.Flush()
	}
	return written, err
}
