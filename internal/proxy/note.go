package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/getlatentic/gusto/internal/spend"
)

// Recorder is told of each call after it is written to the task's out/proxy.jsonl, with the line it was written at.
// It must not block: the proxy waits for it.
type Recorder interface {
	Record(task string, line int, call spend.Call)
}

// UseRecorder has the proxy tell r of every call it writes down. The file is written first and whatever r does
// cannot change that, so a proxy with no recorder, or one whose recorder fails, still keeps its record.
func (p *Proxy) UseRecorder(r Recorder) {
	if r == nil {
		p.recorder.Store(nil)
		return
	}
	p.recorder.Store(&r)
}

// note writes a call to the task's out/proxy.jsonl and counts it in the ledger.
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
	number, written := p.append(job, where, line)
	if written {
		p.tell(job, number, call)
	}
}

// append adds a line to the task's record and says which line it is, counted from zero. The first time this
// process sees a task it counts the lines the file already has.
func (p *Proxy) append(job, where string, line []byte) (number int, written bool) {
	p.guard.Lock()
	defer p.guard.Unlock()
	file, err := os.OpenFile(where, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Error("the proxy could not write down a call", "job", job, "error", err)
		return 0, false
	}
	defer file.Close()
	number, known := p.lines[job]
	if !known {
		if number, err = countLines(where); err != nil {
			slog.Error("the proxy could not count the lines of a task's record", "job", job, "error", err)
			return 0, false
		}
	}
	if _, err := fmt.Fprintf(file, "%s\n", line); err != nil {
		slog.Error("the proxy could not write down a call", "job", job, "error", err)
		return 0, false
	}
	if p.lines == nil {
		p.lines = map[string]int{}
	}
	p.lines[job] = number + 1
	return number, true
}

// tell hands a written call to the recorder. A recorder that panics costs the index a row, which a repair
// reads back from the file, and costs the task nothing.
func (p *Proxy) tell(job string, number int, call spend.Call) {
	recorder := p.recorder.Load()
	if recorder == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("the usage recorder failed", "job", job, "error", recovered)
		}
	}()
	(*recorder).Record(job, number, call)
}

// countLines is how many lines a file has, a last one without its line feed included, and zero for a file not made yet.
func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()
	buffer := make([]byte, 64*1024)
	count, last := 0, byte('\n')
	for {
		n, err := file.Read(buffer)
		count += bytes.Count(buffer[:n], []byte{'\n'})
		if n > 0 {
			last = buffer[n-1]
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if last != '\n' {
		count++
	}
	return count, nil
}

// refuse answers a call a cap stopped, in the error shape an OpenAI-compatible
// client already prints, so the reason reaches the agent's own transcript.
// 402 rather than 429, because a client retries a 429 and a spent cap does not
// come back by waiting.
func (p *Proxy) refuse(w http.ResponseWriter, job string, call spend.Call) {
	p.note(job, call)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"message": "gusto stopped this task: " + call.Refused.Reason,
			"type":    "gusto_cap",
			"code":    call.Refused.Cap + "_cap",
		},
	})
}

// relay hands the container each piece of an answer as it arrives. A streamed
// answer held in a buffer until it fills is an agent waiting on a model that
// already spoke.
type relay struct{ to http.ResponseWriter }

func (r relay) Write(given []byte) (int, error) {
	written, err := r.to.Write(given)
	if flusher, ok := r.to.(http.Flusher); ok {
		flusher.Flush()
	}
	return written, err
}
