package roomwork

import (
	"io/fs"
	"syscall"
)

func stampOf(info fs.FileInfo) stamp {
	if raw, ok := info.Sys().(*syscall.Stat_t); ok {
		return stamp{change: raw.Ctim.Nano(), inode: raw.Ino}
	}
	return stamp{}
}
