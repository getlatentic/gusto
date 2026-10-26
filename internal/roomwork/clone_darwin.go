package roomwork

import "golang.org/x/sys/unix"

// cloneFile makes to share the data blocks of from (APFS clonefile). It fails where the file system cannot.
func cloneFile(from, to string) error { return unix.Clonefile(from, to, unix.CLONE_NOFOLLOW) }
