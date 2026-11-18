package apps

import "strings"

// unixTarget begins a Target that is a unix socket on this machine, where an app in a container with no network is reached.
const unixTarget = "unix:"

// SocketTarget is the Target of an app reached through the socket at path.
func SocketTarget(path string) string { return unixTarget + path }

// Socket is the path of the socket the app is reached through, and false when it is reached at an address.
func (a App) Socket() (string, bool) {
	return strings.CutPrefix(a.Target, unixTarget)
}

// Dialed is the address a request to the app names: where it is reached, or for an app on a socket, which is reached whatever
// the address says, the host it listens for.
func (a App) Dialed() string {
	if _, onSocket := a.Socket(); onSocket {
		return a.Host
	}
	return a.Target
}
