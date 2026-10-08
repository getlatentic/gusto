package sandbox

import (
	"context"
	"path/filepath"

	"github.com/getlatentic/haas/internal/job"
)

// InsideSocket is where a sealed container finds the proxy's socket, and
// InsideProxy is the address the runner relays to it from. Both are inside the
// box, so they are the same for every job.
const (
	InsideSocket = "/run/haas/proxy.sock"
	InsideProxy  = "127.0.0.1:8041"
)

// Sealed runs a job in a container with no network of its own. The only way
// out is the proxy's socket, mounted in, which the runner inside relays to from
// loopback. The model is reached through it, and so is every host the
// allowlist names; the container has no route to anything else, because it has
// no route at all.
//
// The repository is cloned before the container starts, so the box needs no
// route to it either.
func Sealed(dir job.Dir, socket, token string, mounts ...Mount) (int, error) {
	return SealedContext(context.Background(), dir, socket, token, mounts...)
}

// SealedContext is Sealed that stops the container when its context ends.
func SealedContext(ctx context.Context, dir job.Dir, socket, token string, mounts ...Mount) (int, error) {
	absolute, err := filepath.Abs(dir.Path())
	if err != nil {
		return 0, err
	}
	if _, err := ready(absolute); err != nil {
		return 0, err
	}
	args, err := sealedArgv(absolute, socket, token, mounts)
	if err != nil {
		return 0, err
	}
	return start(ctx, dir, args)
}

func sealedArgv(absolute, socket, token string, mounts []Mount) ([]string, error) {
	socket, err := filepath.Abs(socket)
	if err != nil {
		return nil, err
	}
	// A proxy URL carries the token as its password, which a client sends as
	// Proxy-Authorization. The model is on loopback and must not be asked for
	// through the proxy URL, which would send it as a host to allow.
	through := "http://haas:" + token + "@" + InsideProxy
	network := []string{
		"--network=none",
		"-v", socket + ":" + InsideSocket,
		"-e", "HAAS_PROXY_SOCKET=" + InsideSocket,
		"-e", "HAAS_MODEL_URL=http://" + InsideProxy + "/model/v1",
		"-e", "HAAS_MODEL_TOKEN=" + token,
		"-e", "HTTPS_PROXY=" + through, "-e", "https_proxy=" + through,
		"-e", "HTTP_PROXY=" + through, "-e", "http_proxy=" + through,
		"-e", "NO_PROXY=127.0.0.1,localhost", "-e", "no_proxy=127.0.0.1,localhost",
	}
	return argv(absolute, network, mounts)
}
