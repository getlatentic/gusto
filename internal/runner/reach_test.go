package runner

import (
	"net"
	"strings"
	"testing"
)

func TestAModelThatDoesNotAnswerIsNamedBeforeTheHarnessStarts(t *testing.T) {
	// A port nothing listens on: take one, then give it back.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + address + "/v1"
	err = Reachable(endpoint)
	if err == nil {
		t.Fatal("a closed port should not read as a model")
	}
	if !strings.Contains(err.Error(), endpoint) {
		t.Fatalf("the error should name the endpoint, got %v", err)
	}
}

func TestAModelThatAnswersPasses(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := Reachable("http://" + listener.Addr().String() + "/v1"); err != nil {
		t.Fatal(err)
	}
}
