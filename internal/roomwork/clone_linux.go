package roomwork

import (
	"os"

	"golang.org/x/sys/unix"
)

// cloneFile makes to share the data blocks of from (FICLONE on btrfs, xfs and the like). It fails where the file system cannot.
func cloneFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	return os.Chmod(to, info.Mode().Perm())
}
