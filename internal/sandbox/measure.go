package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DiskUse is how many bytes a folder takes on disk. It is counted as root in a container that has no network and the folder
// read-only, so that what an agent left unreadable to this server's own user is counted too: a limit a chmod gets around
// limits nothing.
func (c Containers) DiskUse(ctx context.Context, dir string) (int64, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return 0, err
	}
	if err := c.EnsureImage(ctx); err != nil {
		return 0, err
	}
	args := []string{"run", "--rm", "--network=none", "--read-only", "-v", absolute + ":/measured:ro", "--entrypoint", "du"}
	args = append(args, c.Limits()...)
	args = append(args, "--cap-drop=ALL", "--cap-add=DAC_OVERRIDE", "--cap-add=DAC_READ_SEARCH", "--security-opt=no-new-privileges",
		c.Image, "-sk", "/measured")
	answer, err := exec.CommandContext(ctx, c.Runtime, args...).Output()
	if err != nil {
		return 0, fmt.Errorf("could not count what %s takes: %w", absolute, err)
	}
	fields := strings.Fields(string(answer))
	if len(fields) == 0 {
		return 0, fmt.Errorf("counting what %s takes gave no answer", absolute)
	}
	kilobytes, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || kilobytes < 0 {
		return 0, fmt.Errorf("counting what %s takes gave %q", absolute, fields[0])
	}
	return kilobytes * 1024, nil
}
