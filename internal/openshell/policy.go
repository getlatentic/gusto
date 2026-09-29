package openshell

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Policy is the OpenShell policy for a job, as JSON, which a YAML reader takes.
//
// Everything is denied that is not named. The sandbox may read the system
// paths and write its workspace, /tmp and the harness's own directories. It may
// reach the haas proxy, which holds the model credential, and the hosts the
// operator allowed, and nothing else. A name the operator wrote as a wildcard
// needs three labels in OpenShell, and an entry that cannot be translated is
// refused here and not dropped.
func Policy(cfg Config, modelURL string, writable []string) ([]byte, error) {
	address, err := url.Parse(modelURL)
	if err != nil || address.Hostname() == "" {
		return nil, fmt.Errorf("%q is not an address the sandbox can reach the model proxy at", modelURL)
	}
	port, err := strconv.Atoi(portOf(address))
	if err != nil {
		return nil, fmt.Errorf("%q names no port", modelURL)
	}
	rules := map[string]any{
		"haas_model_proxy": rule([]endpoint{{Host: address.Hostname(), Ports: []int{port}}}),
	}
	for i, entry := range cfg.Allowed {
		endpoints, err := translate(entry)
		if err != nil {
			return nil, err
		}
		rules[fmt.Sprintf("allowed_%d", i+1)] = rule(endpoints)
	}
	policy := map[string]any{
		"version":           1,
		"filesystem_policy": filesystem(writable),
		"landlock":          map[string]string{"compatibility": "hard_requirement"},
		"network_policies":  rules,
	}
	return json.MarshalIndent(policy, "", "  ")
}

// filesystem is what a sandbox may read and write. A policy that names no
// system path leaves the sandbox unable to start its own supervisor, and one
// with no network rule gets none of the baseline paths the gateway would add,
// /dev/null among them, so these are always there.
func filesystem(writable []string) map[string]any {
	return map[string]any{
		"include_workdir": false,
		"read_only":       []string{"/usr", "/lib", "/bin", "/etc", "/proc", "/dev/urandom", "/var/log"},
		"read_write":      append([]string{"/sandbox", "/tmp", "/dev/null"}, writable...),
	}
}

// Denying policy is a policy that lets a sandbox start and reach nothing.
func denying() ([]byte, error) {
	return json.Marshal(map[string]any{"version": 1, "filesystem_policy": filesystem(nil), "network_policies": map[string]any{}})
}

type endpoint struct {
	Host  string
	Ports []int
}

func portOf(address *url.URL) string {
	if port := address.Port(); port != "" {
		return port
	}
	if address.Scheme == "https" {
		return "443"
	}
	return "80"
}

func rule(endpoints []endpoint) map[string]any {
	var listed []map[string]any
	for _, each := range endpoints {
		listed = append(listed, map[string]any{"host": each.Host, "ports": each.Ports, "protocol": "tcp"})
	}
	// Any program in the sandbox may use the rule: the harness is a different
	// program in each image, and the rule is about where it may go.
	return map[string]any{"endpoints": listed, "binaries": []map[string]string{{"path": "/**"}}}
}

// translate reads one entry of HAAS_ALLOWED_HOSTS: a host, which allows ports 80
// and 443, host:port for one port, and *.domain for every name under it.
func translate(entry string) ([]endpoint, error) {
	entry = strings.ToLower(strings.TrimSpace(entry))
	host, portText, err := net.SplitHostPort(entry)
	ports := []int{80, 443}
	if err != nil {
		host = entry
	} else {
		port, convErr := strconv.Atoi(portText)
		if convErr != nil {
			return nil, fmt.Errorf("%q: the port is not a number", entry)
		}
		ports = []int{port}
	}
	if host == "" {
		return nil, fmt.Errorf("%q names no host", entry)
	}
	if domain, wild := strings.CutPrefix(host, "*."); wild && strings.Count(domain, ".") < 1 {
		return nil, fmt.Errorf("%q: a wildcard needs a name with at least three labels in a sandbox's policy, such as *.example.com", entry)
	}
	return []endpoint{{Host: host, Ports: ports}}, nil
}
