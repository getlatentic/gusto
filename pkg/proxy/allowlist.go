package proxy

import (
	"net"
	"strings"
)

// Allowlist is the hosts a task may reach besides its model. It is the
// operator's configuration, and an empty one reaches nothing: a host is refused
// unless it is named.
type Allowlist struct {
	entries []string
}

// ParseAllowlist reads a comma-separated list. An entry is a host, which
// allows its ports 80 and 443, or host:port for one port. A leading "*."
// allows every name under a domain and not the domain itself.
func ParseAllowlist(text string) Allowlist {
	var list Allowlist
	for _, entry := range strings.Split(text, ",") {
		if entry = strings.ToLower(strings.TrimSpace(entry)); entry != "" {
			list.entries = append(list.entries, entry)
		}
	}
	return list
}

// Permits says whether host:port is named. The port is always compared, so a
// named web host does not open its database.
func (a Allowlist) Permits(address string) bool {
	host, port, err := net.SplitHostPort(strings.ToLower(address))
	if err != nil {
		return false
	}
	for _, entry := range a.entries {
		if names(entry, host, port) {
			return true
		}
	}
	return false
}

// Hosts is the list as it was configured, for a person reading what is allowed.
func (a Allowlist) Hosts() []string { return a.entries }

func names(entry, host, port string) bool {
	wanted, wantedPort, err := net.SplitHostPort(entry)
	if err != nil {
		wanted = entry
		if port != "80" && port != "443" {
			return false
		}
	} else if port != wantedPort {
		return false
	}
	if domain, found := strings.CutPrefix(wanted, "*."); found {
		return strings.HasSuffix(host, "."+domain)
	}
	return host == wanted
}
