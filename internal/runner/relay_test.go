package runner

import (
	"bufio"
	"net"
	"path/filepath"
	"testing"
)

// A sealed box reaches the proxy only through the socket mounted into it, so
// what the harness sends to loopback has to arrive at the socket whole, and the
// answer has to come back the same way.
func TestTheRelayCarriesLoopbackToTheSocket(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "p.sock")
	far, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer far.Close()
	go func() {
		conn, err := far.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadString('\n')
		conn.Write([]byte("heard " + line))
	}()
	near, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := relay(near, socket)
	defer stop()

	conn, err := net.Dial("tcp", near.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("hello\n"))
	answer, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || answer != "heard hello\n" {
		t.Fatalf("answer %q, %v", answer, err)
	}
}

func TestABoxThatIsNotSealedRelaysNothing(t *testing.T) {
	t.Setenv("HAAS_PROXY_SOCKET", "")
	stop, err := relayOut()
	if err != nil {
		t.Fatal(err)
	}
	stop()
}
