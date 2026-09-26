package doctor

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/getlatentic/haas/internal/sandbox"
)

// podmanRuntimeDirectory checks rootless podman can reach the directory it
// talks to conmon through. A systemd unit with ProtectHome hides /run/user, and
// podman then refuses every container with exit 125 and a line about symlinks,
// which is the failure a whole deploy once took to find.
func podmanRuntimeDirectory(machine Machine) Verdict {
	if filepath.Base(sandbox.Runtime()) != "podman" {
		return skip("the container command is " + sandbox.Runtime() + ", not podman")
	}
	if machine.User() == 0 {
		return skip("podman runs as root here, and root keeps its runtime directory under /run")
	}
	dir := runtimeDirectory(machine.User())
	if _, err := os.ReadDir(dir); err != nil {
		return fail("podman cannot reach its runtime directory: "+err.Error(),
			"set XDG_RUNTIME_DIR to a directory this user owns (`loginctl enable-linger` makes /run/user/<uid>), and under systemd leave out ProtectHome, which hides /run/user")
	}
	return pass("podman's runtime directory " + dir + " is reachable")
}

// runtimeDirectory is where rootless podman looks: XDG_RUNTIME_DIR when it is
// set, and the login session's directory when it is not.
func runtimeDirectory(user int) string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return filepath.Join("/run/user", strconv.Itoa(user))
}
