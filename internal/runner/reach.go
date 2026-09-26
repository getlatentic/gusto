package runner

import (
	"fmt"
	"net"
	"net/url"
	"time"
)

// Reachable checks a local model answers before the harness starts.
//
// An unreachable endpoint otherwise surfaces as the harness's own error, minutes
// later, in the middle of a transcript. The common cause is a model server bound
// to loopback, which no container can reach.
func Reachable(endpoint string) error {
	address, err := dialable(endpoint)
	if err != nil {
		return err
	}
	connection, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return fmt.Errorf("no model answers at %s: %w", endpoint, err)
	}
	return connection.Close()
}

func dialable(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%s is not an address a runner can reach", endpoint)
	}
	if parsed.Port() != "" {
		return parsed.Host, nil
	}
	if parsed.Scheme == "https" {
		return parsed.Hostname() + ":443", nil
	}
	return parsed.Hostname() + ":80", nil
}
