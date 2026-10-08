//go:build !windows

package web

import (
	"os"
	"syscall"
)

// matchOwner gives a file or folder the owner of the agent's folder, when the server may. A box runs the agent as another user,
// and a file this server made as itself is one the agent could read and not change, so the owner is the folder's. Where the
// server may not change an owner it does not, and the next run hands the folder over as it always does.
func matchOwner(path, root string) {
	info, err := os.Stat(root)
	if err != nil {
		return
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		_ = os.Lchown(path, int(stat.Uid), int(stat.Gid))
	}
}
