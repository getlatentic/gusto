// Package rules is what a runner checks before it starts anything.
//
// One rule today, and it is the one that cannot be added after the fact without
// changing where tasks already ran: restricted code only talks to a model in the
// same building. Spend caps and model-to-repository rules join it here.
package rules

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/getlatentic/gusto/pkg/job"
)

// Allows refuses a task whose code may not leave against a model endpoint that
// is not local. The message names the endpoint, because whoever wrote the task is
// the one who has to fix it.
func Allows(spec job.Spec, endpoint string) error {
	if spec.Sensitivity != job.Restricted {
		return nil
	}
	// No endpoint means no model: there is nowhere for the code to go.
	if endpoint == "" || Local(endpoint) {
		return nil
	}
	return fmt.Errorf("restricted code, so the model has to be local: %s is not", endpoint)
}

// Local reports whether an endpoint is on this machine or on a private network
// the company runs.
func Local(endpoint string) bool {
	host := endpoint
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Host != "" {
		host = parsed.Hostname()
	} else if h, _, err := net.SplitHostPort(endpoint); err == nil {
		host = h
	}
	switch host {
	case "localhost", "host.docker.internal":
		return true
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && (address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast())
}
