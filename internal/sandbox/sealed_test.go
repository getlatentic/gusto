package sandbox

import (
	"slices"
	"strings"
	"testing"
)

// Whether this holds on podman and docker has to be measured on the box. What
// can be held here is that the command asks for it.
func TestASealedContainerHasNoNetworkButTheProxysSocket(t *testing.T) {
	args, err := sealedArgv("/jobs/a", "/jobs/.proxy.sock", "a-token", Owner{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"--network=none",
		"/jobs/.proxy.sock:" + InsideSocket,
		"GUSTO_PROXY_SOCKET=" + InsideSocket,
		"GUSTO_MODEL_URL=http://127.0.0.1:8041/model/v1",
		"GUSTO_MODEL_TOKEN=a-token",
		"HTTPS_PROXY=http://gusto:a-token@127.0.0.1:8041",
		"http_proxy=http://gusto:a-token@127.0.0.1:8041",
		"NO_PROXY=127.0.0.1,localhost",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("want %q in %v", want, args)
		}
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--add-host") || strings.HasPrefix(arg, "--network=") && arg != "--network=none" {
			t.Fatalf("a sealed container was given %q", arg)
		}
	}
	if args[len(args)-2] != "inside" || args[len(args)-1] != "/job" {
		t.Fatalf("the runner should still be what runs: %v", args)
	}
}

func TestAContainerWithoutAProxyKeepsTheRuntimesNetwork(t *testing.T) {
	args, err := argv("/jobs/a", []string{"-e", "GUSTO_MODEL_URL=http://host.docker.internal:11434/v1"}, Owner{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--network") {
			t.Fatalf("an unsealed container was given %q", arg)
		}
	}
}
