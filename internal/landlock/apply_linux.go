package landlock

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	ruleNetPort = 2

	dirOnly = unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM | unix.LANDLOCK_ACCESS_FS_REFER
)

// ABI is the version of Landlock this kernel has: 1 from Linux 5.13, 4 from 6.7, which is the first that limits the network.
func ABI() (int, error) {
	abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	switch errno {
	case 0:
		return int(abi), nil
	case unix.ENOSYS:
		return 0, errors.New("this kernel has no Landlock (it came in Linux 5.13)")
	case unix.EOPNOTSUPP:
		return 0, errors.New("Landlock is built into this kernel and turned off")
	}
	return 0, errno
}

// handledFS is the file accesses a kernel of this version can limit, which are all of them it knows.
func handledFS(abi int) uint64 {
	mask := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM)
	if abi >= 2 {
		mask |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		mask |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	return mask
}

const readOnly = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR

// Apply restricts the calling process, and everything it starts, to the policy. It cannot be undone. A folder that does not exist is
// left out: nothing can be reached through it.
func Apply(p Policy) error {
	runtime.LockOSThread()
	abi, err := ABI()
	if err != nil {
		return err
	}
	if p.ProxyPort > 0 && abi < 4 {
		return fmt.Errorf("this kernel's Landlock is version %d, and limiting the network needs 4 (Linux 6.7)", abi)
	}
	fs := handledFS(abi)
	attr := unix.LandlockRulesetAttr{Access_fs: fs}
	size := uintptr(8)
	if p.ProxyPort > 0 {
		attr.Access_net = unix.LANDLOCK_ACCESS_NET_CONNECT_TCP
		size = 16
	}
	ruleset, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), size, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	defer unix.Close(int(ruleset))
	for access, folders := range map[uint64][]string{readOnly & fs: append(append([]string{}, System...), p.Read...), fs: append(append([]string{}, Scratch...), p.Write...)} {
		for _, folder := range folders {
			if err := allow(int(ruleset), folder, access&fs); err != nil {
				return err
			}
		}
	}
	if p.ProxyPort > 0 {
		rule := struct{ Access, Port uint64 }{unix.LANDLOCK_ACCESS_NET_CONNECT_TCP, uint64(p.ProxyPort)}
		if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, ruleset, ruleNetPort, uintptr(unsafe.Pointer(&rule)), 0, 0, 0); errno != 0 {
			return fmt.Errorf("landlock_add_rule: %w", errno)
		}
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, ruleset, 0, 0); errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}
	return nil
}

// allow lets the process do access beneath folder.
func allow(ruleset int, folder string, access uint64) error {
	fd, err := unix.Open(folder, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, os.ErrPermission) {
			return nil
		}
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		access &^= dirOnly
	}
	rule := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
	if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(ruleset), unix.LANDLOCK_RULE_PATH_BENEATH, uintptr(unsafe.Pointer(&rule)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("landlock_add_rule %s: %w", folder, errno)
	}
	return nil
}
