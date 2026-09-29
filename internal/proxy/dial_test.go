package proxy

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

// listening is a local server and the port it is on.
func listening(t *testing.T) (net.Listener, string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return l, port
}

func answering(addrs ...string) (Resolver, *int) {
	lookups := 0
	return func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		var out []netip.Addr
		for _, a := range addrs {
			out = append(out, netip.MustParseAddr(a))
		}
		return out, nil
	}, &lookups
}

func TestANameThatResolvesInwardIsNotDialled(t *testing.T) {
	_, port := listening(t)
	for name, answer := range map[string][]string{
		"this machine":                 {"127.0.0.1"},
		"this machine over IPv6":       {"::1"},
		"a cloud's metadata service":   {"169.254.169.254"},
		"link-local over IPv6":         {"fe80::1"},
		"an IPv4 address written IPv6": {"::ffff:169.254.169.254"},
		"nothing at all":               {"0.0.0.0"},
	} {
		resolve, _ := answering(answer...)
		p := &Proxy{resolve: resolve}
		conn, err := p.dial(context.Background(), "tcp", "allowed.example.com:"+port)
		if err == nil {
			conn.Close()
			t.Errorf("%s: a job was connected", name)
		}
	}
}

// The connection is made to an address from the one answer that was checked,
// so a name that answers differently the second time cannot be rebound in
// between.
func TestANameIsResolvedOnceForAConnection(t *testing.T) {
	_, port := listening(t)
	resolve, lookups := answering("169.254.169.254", "127.0.0.2")
	p := &Proxy{resolve: resolve}
	if conn, err := p.dial(context.Background(), "tcp", "allowed.example.com:"+port); err == nil {
		conn.Close()
		t.Fatal("a name whose every answer is inward was connected")
	}
	if *lookups != 1 {
		t.Errorf("the name was resolved %d times for one connection", *lookups)
	}
}

func TestAnAddressTheOperatorWroteIsTheirs(t *testing.T) {
	_, port := listening(t)
	p := &Proxy{}
	conn, err := p.dial(context.Background(), "tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("a loopback address written as a literal was refused: %v", err)
	}
	conn.Close()
	if conn, err := p.dial(context.Background(), "tcp", "169.254.169.254:80"); err == nil {
		conn.Close()
		t.Error("a link-local address was dialled because it was written as a literal")
	}
}

func TestOnlyWhatIsInwardIsRefused(t *testing.T) {
	for address, refused := range map[string]bool{
		"8.8.8.8": false, "10.1.2.3": false, "172.16.0.9": false, "192.168.1.1": false,
		"127.0.0.1": true, "::1": true, "169.254.169.254": true, "fe80::1": true, "0.0.0.0": true, "224.0.0.1": true,
	} {
		if got := unreachable(netip.MustParseAddr(address), false) != ""; got != refused {
			t.Errorf("%s: refused=%v, want %v", address, got, refused)
		}
	}
}
