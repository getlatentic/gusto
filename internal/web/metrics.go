package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/retention"
	"github.com/getlatentic/gusto/pkg/proxy"
)

// Health is what /metrics and /readyz report on besides the index: the release
// this is, where a deploy leaves its result, and what the proxy has counted.
type Health struct {
	Release string
	// DeployResult is the file a deploy writes when it ends, or empty.
	DeployResult string
	// Proxy is the proxy's totals, or nil when the server has no proxy.
	Proxy func() proxy.Counts
}

// UseHealth says what the release is and where the rest of what to report on is.
func (s *Server) UseHealth(h Health) {
	s.health = h
	s.startedAt = time.Now()
}

// diskFor is how long the size of the jobs directory is kept: measuring it
// walks every job, and a scrape every fifteen seconds should not.
const diskFor = time.Minute

type diskUse struct {
	mu       sync.Mutex
	usage    retention.Usage
	measured time.Time
}

func (d *diskUse) get(s *Server) retention.Usage {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.measured.IsZero() || time.Since(d.measured) > diskFor {
		d.usage, d.measured = retention.Measure(s.index.Root(), s.index.List()), time.Now()
	}
	return d.usage
}

// metrics writes what an operator watches, in the Prometheus text format, for a
// caller that holds a token that may read it.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	out := &exposition{w: w}
	stats := s.index.Stats(time.Now())

	out.gauge("gusto_build_info", "The release this server is.", 1, label{"release", or(s.health.Release, "unknown")})
	out.gauge("gusto_started_timestamp_seconds", "When this server started.", float64(s.startedAt.Unix()))

	out.header("gusto_jobs", "gauge", "Jobs the server holds, by state.")
	for _, state := range []jobs.State{jobs.Queued, jobs.Running, jobs.Passed, jobs.Failed, jobs.Interrupted} {
		out.sample("gusto_jobs", float64(stats.ByState[state]), label{"state", string(state)})
	}
	out.gauge("gusto_queue_depth", "Jobs waiting to start.", float64(stats.ByState[jobs.Queued]))
	out.gauge("gusto_queue_oldest_wait_seconds", "How long the job queued longest has waited.", float64(stats.OldestWaitSeconds))
	out.gauge("gusto_queue_stuck_seconds", "How long the longest-waiting job has waited that no limit, busy agent or full pool holds back. Near zero while the queue moves.", float64(stats.StuckSeconds))
	out.gauge("gusto_jobs_running", "Jobs running now.", float64(stats.ByState[jobs.Running]))
	out.gauge("gusto_runner_slots", "How many jobs the server runs at once.", float64(stats.Slots))
	if s.queue != nil {
		out.gauge("gusto_runner_workers_alive", "Workers running. Fewer than the slots means the queue takes jobs it will not start.", float64(s.queue.Alive()))
	}
	out.header("gusto_jobs_finished_total", "counter", "Jobs that ended since this server started, by how.")
	for _, state := range []jobs.State{jobs.Passed, jobs.Failed, jobs.Interrupted} {
		out.sample("gusto_jobs_finished_total", float64(stats.Finished[state]), label{"state", string(state)})
	}
	out.gauge("gusto_last_job_started_timestamp_seconds", "When a job last started, since this server started.", float64(stats.LastStarted))
	out.gauge("gusto_last_job_finished_timestamp_seconds", "When a job last ended, since this server started.", float64(stats.LastFinished))

	s.diskMetrics(out)
	if s.health.Proxy != nil {
		counts := s.health.Proxy()
		out.counter("gusto_proxy_calls_total", "Calls the model proxy has handled since it started.", float64(counts.Calls))
		out.counter("gusto_proxy_errors_total", "Of them, calls the model's side failed: no answer, or a 5xx.", float64(counts.Errors))
		out.counter("gusto_proxy_refused_total", "Of them, calls a spending cap stopped.", float64(counts.Refused))
	}
	if result, found := deployResult(s.health.DeployResult); found {
		ok := 0.0
		if result.OK {
			ok = 1
		}
		out.gauge("gusto_last_deploy_ok", "1 when the last deploy ended with the new version serving, 0 when it failed or rolled back.", ok, label{"release", result.Release})
		out.gauge("gusto_last_deploy_timestamp_seconds", "When the last deploy ended.", float64(result.At))
	}
}

func (s *Server) diskMetrics(out *exposition) {
	root := s.index.Root()
	if free, total, err := volume(root); err == nil {
		out.gauge("gusto_disk_free_bytes", "Free space on the volume the jobs are kept on.", float64(free), label{"store", "jobs"})
		out.gauge("gusto_disk_total_bytes", "Size of the volume the jobs are kept on.", float64(total), label{"store", "jobs"})
	}
	used := s.disk.get(s)
	out.gauge("gusto_jobs_disk_bytes", "Bytes the jobs take, workspaces included. Measured at most once a minute.", float64(used.Bytes))
	out.gauge("gusto_jobs_workspace_bytes", "Of them, the workspaces.", float64(used.Workspaces))
	if used.Oldest != nil {
		out.gauge("gusto_oldest_finished_job_timestamp_seconds", "When the finished job kept longest ended.", float64(used.Oldest.Ended))
	}
}

// DeployResult is what a deploy writes when it ends.
type DeployResult struct {
	Release string `json:"release"`
	OK      bool   `json:"ok"`
	At      int64  `json:"at"`
}

func deployResult(path string) (DeployResult, bool) {
	if path == "" {
		return DeployResult{}, false
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return DeployResult{}, false
	}
	var result DeployResult
	return result, json.Unmarshal(text, &result) == nil
}

// exposition writes the Prometheus text format.
type exposition struct{ w io.Writer }

type label struct{ name, value string }

func (e *exposition) header(name, kind, help string) {
	fmt.Fprintf(e.w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

func (e *exposition) sample(name string, value float64, labels ...label) {
	fmt.Fprintf(e.w, "%s%s %s\n", name, rendered(labels), number(value))
}

func (e *exposition) gauge(name, help string, value float64, labels ...label) {
	e.header(name, "gauge", help)
	e.sample(name, value, labels...)
}

func (e *exposition) counter(name, help string, value float64, labels ...label) {
	e.header(name, "counter", help)
	e.sample(name, value, labels...)
}

func number(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d", int64(value))
	}
	return fmt.Sprintf("%g", value)
}

// rendered is the {name="value"} part, with the escapes the format asks for.
func rendered(labels []label) string {
	if len(labels) == 0 {
		return ""
	}
	sort.Slice(labels, func(a, b int) bool { return labels[a].name < labels[b].name })
	parts := make([]string, len(labels))
	escape := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	for i, each := range labels {
		parts[i] = each.name + `="` + escape.Replace(each.value) + `"`
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// ready answers 200 when the server can do its work, and 503 with what is wrong
// when it cannot. /healthz says the process is there; this says it is fit to be
// promoted: its workers are running and it can write where it keeps jobs.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	var wrong []string
	if s.queue != nil && s.queue.Alive() < s.queue.Slots() {
		wrong = append(wrong, fmt.Sprintf("%d of %d workers are running", s.queue.Alive(), s.queue.Slots()))
	}
	if err := writable(s.index.Root()); err != nil {
		wrong = append(wrong, "the jobs directory cannot be written: "+err.Error())
	}
	if len(wrong) > 0 {
		http.Error(w, "not ready: "+strings.Join(wrong, "; ")+"\n", http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintln(w, "ready")
}

// writable proves a directory can be written by writing to it.
func writable(dir string) error {
	file, err := os.CreateTemp(dir, ".ready-")
	if err != nil {
		return err
	}
	file.Close()
	return os.Remove(filepath.Join(dir, filepath.Base(file.Name())))
}
