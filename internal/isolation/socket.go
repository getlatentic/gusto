package isolation

import (
	"errors"
	"net"
	"os"
	"path/filepath"
)

// SocketMount is the test of whether a container runtime can be given a unix socket of this computer, made in gusto's own folder
// where the jobs' socket will be. It starts a container of the image that does nothing with the socket mounted, and says what the
// runtime answered when it cannot.
func SocketMount(run func(program string, args ...string) (string, bool)) func(runtime, image string) error {
	return func(runtime, image string) error {
		dir, err := os.MkdirTemp(".", ".probe-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if dir, err = filepath.Abs(dir); err != nil {
			return err
		}
		socket := filepath.Join(dir, "s")
		listener, err := net.Listen("unix", socket)
		if err != nil {
			return err
		}
		defer listener.Close()
		if out, ok := run(runtime, "run", "--rm", "--entrypoint", "true", "--mount", "type=bind,src="+socket+",dst=/probe.sock", image); !ok {
			return errors.New(out)
		}
		return nil
	}
}
