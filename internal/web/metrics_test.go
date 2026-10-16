package web

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/proxy"
)

var sample = regexp.MustCompile(`^([a-z_:][a-z0-9_:]*)(\{[^}]*\})? (-?[0-9.e+-]+)$`)

// scraped parses what /metrics said the way a scraper does, and fails on
// anything a scraper would refuse: a sample with no declared type, a type
// declared twice, a value that is not a number.
func scraped(t *testing.T, body string) map[string]float64 {
	t.Helper()
	typed := map[string]bool{}
	values := map[string]float64{}
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if kind, found := strings.CutPrefix(line, "# TYPE "); found {
			name := strings.Fields(kind)[0]
			if typed[name] {
				t.Errorf("%s is declared twice", name)
			}
			typed[name] = true
			continue
		}
		if strings.HasPrefix(line, "# HELP ") {
			continue
		}
		parts := sample.FindStringSubmatch(line)
		if parts == nil {
			t.Fatalf("not a sample: %q", line)
		}
		if !typed[parts[1]] {
			t.Errorf("%s has a sample before its type", parts[1])
		}
		value, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		values[parts[1]+parts[2]] = value
	}
	return values
}

func TestMetricsReportTheQueueTheDiskAndTheProxy(t *testing.T) {
	here := open(t)
	here.server.UseHealth(Health{Release: "abc1234", Proxy: func() proxy.Counts { return proxy.Counts{Calls: 40, Errors: 3, Refused: 2} }})
	for id, state := range map[string]jobs.State{"a": jobs.Queued, "b": jobs.Queued, "c": jobs.Running, "d": jobs.Passed, "e": jobs.Failed} {
		waitingJob(t, here, id, "amy@acme.test", state, int64(len(id))+int64(state[0]))
	}
	got := here.call("GET", "/metrics", password)
	if got.Code != http.StatusOK || !strings.HasPrefix(got.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("%d %q", got.Code, got.Header().Get("Content-Type"))
	}
	values := scraped(t, got.Body.String())
	for series, want := range map[string]float64{
		`gusto_build_info{release="abc1234"}`: 1,
		`gusto_jobs{state="queued"}`:          2,
		`gusto_jobs{state="running"}`:         1,
		`gusto_jobs{state="passed"}`:          1,
		`gusto_jobs{state="failed"}`:          1,
		`gusto_queue_depth`:                   2,
		`gusto_jobs_running`:                  1,
		`gusto_proxy_calls_total`:             40,
		`gusto_proxy_errors_total`:            3,
		`gusto_proxy_refused_total`:           2,
	} {
		if values[series] != want {
			t.Errorf("%s is %v, want %v", series, values[series], want)
		}
	}
	for _, series := range []string{"gusto_disk_free_bytes{store=\"jobs\"}", "gusto_disk_total_bytes{store=\"jobs\"}", "gusto_jobs_disk_bytes",
		"gusto_runner_workers_alive", "gusto_runner_slots", "gusto_queue_stuck_seconds", "gusto_started_timestamp_seconds"} {
		if _, found := values[series]; !found {
			t.Errorf("no %s", series)
		}
	}
	if values["gusto_jobs_disk_bytes"] <= 0 {
		t.Error("the jobs take no disk")
	}
	if values[`gusto_disk_free_bytes{store="jobs"}`] <= 0 || values[`gusto_disk_total_bytes{store="jobs"}`] < values[`gusto_disk_free_bytes{store="jobs"}`] {
		t.Error("the volume's size and free space do not make sense")
	}
}

func TestMetricsAreForACallerThatMayReadThem(t *testing.T) {
	here := withTokens(t)
	if got := here.call("GET", "/metrics", ""); got.Code != http.StatusUnauthorized {
		t.Errorf("no credential gave %d", got.Code)
	}
	if got := here.call("GET", "/metrics", password); got.Code != http.StatusUnauthorized {
		t.Errorf("the shared password gave %d on a server with tokens", got.Code)
	}
	if got := here.call("GET", "/metrics", here.secrets["finance"]); got.Code != http.StatusForbidden {
		t.Errorf("a token that reads spending gave %d", got.Code)
	}
	here.signIn(t)
	made := here.ask(t, "POST", "/admin/tokens", map[string][]string{"name": {"prometheus"}, "scope": {"metrics"}})
	secret := regexp.MustCompile(`gusto_[0-9a-f]{8}_[A-Za-z0-9_-]{43}`).FindString(made.Body.String())
	if got := here.call("GET", "/metrics", secret); got.Code != http.StatusOK {
		t.Errorf("a token that reads metrics gave %d", got.Code)
	}
	if got := here.call("GET", "/api/spend", secret); got.Code != http.StatusForbidden {
		t.Errorf("a metrics token read spending: %d", got.Code)
	}
}

func TestMetricsSayHowTheLastDeployEnded(t *testing.T) {
	here := open(t)
	file := filepath.Join(t.TempDir(), "deploy.json")
	here.server.UseHealth(Health{Release: "r", DeployResult: file})
	if _, found := scraped(t, here.call("GET", "/metrics", password).Body.String())["gusto_last_deploy_timestamp_seconds"]; found {
		t.Error("a deploy that left no result was reported")
	}
	os.WriteFile(file, []byte(`{"release":"3ac3db3","ok":false,"at":1790000000}`), 0o644)
	values := scraped(t, here.call("GET", "/metrics", password).Body.String())
	if values[`gusto_last_deploy_ok{release="3ac3db3"}`] != 0 || values["gusto_last_deploy_timestamp_seconds"] != 1790000000 {
		t.Errorf("%v", values)
	}
	os.WriteFile(file, []byte(`{"release":"3ac3db3","ok":true,"at":1790000100}`), 0o644)
	if scraped(t, here.call("GET", "/metrics", password).Body.String())[`gusto_last_deploy_ok{release="3ac3db3"}`] != 1 {
		t.Error("a deploy that worked was reported as failed")
	}
	os.WriteFile(file, []byte(`{not json`), 0o644)
	if _, found := scraped(t, here.call("GET", "/metrics", password).Body.String())["gusto_last_deploy_timestamp_seconds"]; found {
		t.Error("a result that cannot be read was reported")
	}
}

func TestALabelIsEscapedAsTheFormatAsks(t *testing.T) {
	if got := rendered([]label{{"release", "a\"b\\c\nd"}}); got != `{release="a\"b\\c\nd"}` {
		t.Errorf("%s", got)
	}
}

func TestReadyMeansTheServerCanDoItsWork(t *testing.T) {
	here := open(t)
	if got := here.call("GET", "/readyz", ""); got.Code != http.StatusOK {
		t.Fatalf("a working server gave %d: %s", got.Code, got.Body.String())
	}
	if os.Getuid() != 0 {
		root := here.index.Root()
		os.Chmod(root, 0o500)
		defer os.Chmod(root, 0o755)
		got := here.call("GET", "/readyz", "")
		if got.Code != http.StatusServiceUnavailable || !strings.Contains(got.Body.String(), "cannot be written") {
			t.Errorf("a jobs directory that cannot be written gave %d: %s", got.Code, got.Body.String())
		}
	}
	// A queue whose workers are gone takes jobs and never starts them.
	stopped := open(t)
	stopped.server.queue.Stop()
	if got := stopped.call("GET", "/readyz", ""); got.Code != http.StatusServiceUnavailable || !strings.Contains(got.Body.String(), "workers are running") {
		t.Errorf("a queue with no workers gave %d: %s", got.Code, got.Body.String())
	}
	if got := stopped.call("GET", "/healthz", ""); got.Code != http.StatusOK {
		t.Errorf("liveness must not depend on the workers: %d", got.Code)
	}
}
