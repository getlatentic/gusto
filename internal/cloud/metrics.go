package cloud

import (
	"errors"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsPath is where the front door serves its metrics, to its own host only.
const MetricsPath = "/.gusto/metrics"

// Metrics is what one front door process reports: its requests, the organizations' servers it holds by state, how long they
// take to provision, and whether it leads. It has a registry of its own, not the global one. No label holds a person, an
// address, a request id or an organization: a label is a route class, a status class or a state. Every method may be called
// on a nil *Metrics, which counts nothing.
type Metrics struct {
	registry       *prometheus.Registry
	requests       *prometheus.CounterVec
	latency        *prometheus.HistogramVec
	provision      prometheus.Histogram
	provisionFails prometheus.Counter
	demoDeferred   prometheus.Counter
}

// NewMetrics makes the registry, with the process's own Go and process metrics in it.
func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gusto_front_requests_total",
			Help: "Requests by route class and status class."}, []string{"class", "status"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "gusto_front_request_seconds",
			Help: "Request time by route class.", Buckets: prometheus.DefBuckets}, []string{"class"}),
		provision: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "gusto_provision_seconds",
			Help: "How long an organization's server took to provision, ready or failed.", Buckets: []float64{1, 2, 5, 10, 20, 40, 80, 160}}),
		provisionFails: prometheus.NewCounter(prometheus.CounterOpts{Name: "gusto_provision_failures_total",
			Help: "Provisions that failed."}),
		demoDeferred: prometheus.NewCounter(prometheus.CounterOpts{Name: "gusto_demo_ends_deferred_total",
			Help: "Demo endings put off because computers were still to be removed."}),
	}
	m.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests, m.latency, m.provision, m.provisionFails, m.demoDeferred)
	return m
}

// Handler serves the metrics in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// watch reports the controller's organizations by state and its lease.
func (m *Metrics) watch(c *Controller) {
	if m == nil || c == nil {
		return
	}
	m.registry.MustRegister(&controllerCollector{c: c})
}

// provisioned records a provision that ended in a server or a failure. One that waits for room, that found its organization
// gone, or that another gusto cloud server now leads is neither.
func (m *Metrics) provisioned(took time.Duration, err error) {
	if m == nil || errors.Is(err, ErrNoCapacity) || errors.Is(err, ErrRemoved) || errors.Is(err, ErrNotLeading) {
		return
	}
	m.provision.Observe(took.Seconds())
	if err != nil {
		m.provisionFails.Inc()
	}
}

func (m *Metrics) endDeferred() {
	if m != nil {
		m.demoDeferred.Inc()
	}
}

var (
	orgsDesc  = prometheus.NewDesc("gusto_orgs", "Organizations' servers the controller holds, by state.", []string{"state"}, nil)
	leadsDesc = prometheus.NewDesc("gusto_front_leads", "1 when this front door runs the controller, else 0.", nil, nil)
	epochDesc = prometheus.NewDesc("gusto_front_epoch", "The lease epoch this front door leads with, 0 when no lease governs it.", nil, nil)
	allStates = []State{Waiting, Provisioning, Starting, Ready, Failed}
	loopback  = []string{"localhost", "127.0.0.1", "[::1]", "::1"}
)

// controllerCollector reads the controller when the metrics are asked for, so nothing is counted on each change.
type controllerCollector struct{ c *Controller }

func (k *controllerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- orgsDesc
	ch <- leadsDesc
	ch <- epochDesc
}

func (k *controllerCollector) Collect(ch chan<- prometheus.Metric) {
	counts := map[State]int{}
	k.c.mu.Lock()
	for _, held := range k.c.orgs {
		counts[held.runtime.State]++
	}
	k.c.mu.Unlock()
	for _, state := range allStates {
		ch <- prometheus.MustNewConstMetric(orgsDesc, prometheus.GaugeValue, float64(counts[state]), string(state))
	}
	epoch := k.c.epoch.Load()
	leads := 0.0
	if epoch >= 0 {
		leads = 1
	}
	ch <- prometheus.MustNewConstMetric(leadsDesc, prometheus.GaugeValue, leads)
	ch <- prometheus.MustNewConstMetric(epochDesc, prometheus.GaugeValue, float64(max(epoch, 0)))
}

// statusWriter remembers the status a response was given. Unwrap lets a proxied connection still be flushed and taken over.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// observe counts each request and its time under the class it is given when it is done, since the mux names its pattern
// only once it has matched.
func (m *Metrics) observe(next http.Handler, class func(*http.Request) string) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		began := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		name := class(r)
		m.requests.WithLabelValues(name, statusClass(sw.status)).Inc()
		m.latency.WithLabelValues(name).Observe(time.Since(began).Seconds())
	})
}

// statusClass is the first digit of a status with "xx", so a status is five values and not sixty.
func statusClass(status int) string {
	return string(rune('0'+status/100)) + "xx"
}

// onlyHere serves what is behind it to a request that came straight to this machine's loopback, and refuses one that went
// through a proxy or names another host: the metrics are for the agent on the front door's own host.
func onlyHere(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Forwarded") != "" || !slices.Contains(loopback, host) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
