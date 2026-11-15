package cloud

import (
	"net/http"
	"strings"
	"testing"
)

func TestTheFrontDoorReportsItsRequestsOrganizationsAndLease(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		f.server.Wait()
		b.get("/demo/wait")
		b.get("/no/such/page")

		status, text, _ := f.browser().get2(MetricsPath)
		if status != http.StatusOK {
			t.Fatalf("%d\n%s", status, text)
		}
		for _, want := range []string{
			`gusto_front_requests_total{class="POST /demo",status="3xx"} 1`,
			`gusto_front_requests_total{class="unmatched",status="4xx"} 1`,
			`gusto_front_request_seconds_count{class="POST /demo"} 1`,
			`gusto_orgs{state="ready"} 1`,
			`gusto_orgs{state="failed"} 0`,
			`gusto_provision_seconds_count 1`,
			`gusto_provision_failures_total 0`,
			`gusto_front_leads 1`,
			`go_goroutines`,
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the metrics lack %q", want)
			}
		}
		for _, banned := range []string{"ada@acme.com", "Acme", "slug="} {
			if strings.Contains(text, banned) {
				t.Errorf("the metrics carry %q", banned)
			}
		}
	})
}

func TestTheMetricsAreServedToThisMachineOnly(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	b := f.browser()
	for name, header := range map[string]map[string]string{
		"a forwarded request": {"X-Forwarded-For": "203.0.113.9"},
		"another host name":   {"Host": "cloud.test"},
	} {
		if status, _, _ := b.do("GET", MetricsPath, nil, header); status != http.StatusNotFound {
			t.Errorf("%s was served: %d", name, status)
		}
	}
	if status, _, _ := b.get2(MetricsPath); status != http.StatusOK {
		t.Errorf("this machine was refused: %d", status)
	}
}

func TestAFailedProvisionIsCountedAndOneWaitingForRoomIsNot(t *testing.T) {
	m := NewMetrics()
	m.provisioned(0, nil)
	m.provisioned(0, ErrNoCapacity)
	m.provisioned(0, ErrRemoved)
	m.provisioned(0, http.ErrAbortHandler)
	(*Metrics)(nil).provisioned(0, http.ErrAbortHandler)
	f := newFront(t, Settings{}, func(c *Config) { c.Metrics = m })
	_, text, _ := f.browser().get2(MetricsPath)
	for _, want := range []string{"gusto_provision_seconds_count 2", "gusto_provision_failures_total 1"} {
		if !strings.Contains(text, want) {
			t.Errorf("the metrics lack %q:\n%s", want, text)
		}
	}
}
