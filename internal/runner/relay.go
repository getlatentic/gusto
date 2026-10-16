package runner

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/getlatentic/gusto/internal/proxy"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// relayOut is the only way out of a sealed box. The container has no network,
// so the harness asks on loopback and the runner hands each connection to the
// proxy's socket, mounted in from the host. In a box that is not sealed there
// is no socket, and nothing is relayed.
func relayOut() (func(), error) {
	socket := os.Getenv("GUSTO_PROXY_SOCKET")
	if socket == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", sandbox.InsideProxy)
	if err != nil {
		return nil, fmt.Errorf("the way out of this box could not open on %s: %w", sandbox.InsideProxy, err)
	}
	return relay(listener, socket), nil
}

// relay hands every connection listener accepts to the socket, until the
// returned func closes it.
func relay(listener net.Listener, socket string) func() {
	go func() {
		for {
			near, err := listener.Accept()
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err != nil {
				continue
			}
			go pass(near, socket)
		}
	}()
	return func() { _ = listener.Close() }
}

func pass(near net.Conn, socket string) {
	far, err := net.Dial("unix", socket)
	if err != nil {
		near.Close()
		return
	}
	proxy.Splice(near, far)
}
