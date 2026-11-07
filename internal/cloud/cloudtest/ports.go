// Package cloudtest is what the tests of gusto cloud's controller, and of the nodes it starts servers on, have in common.
package cloudtest

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/getlatentic/gusto/internal/filelock"
)

const (
	firstPort = 12000
	blockSize = 200
	blocks    = 190
)

var (
	claim sync.Once
	from  int
	// held keeps the claim for as long as the process runs, when the operating system lets it go.
	held *filelock.Held
)

// PortsFrom is the first of a block of ports that no other test process on this machine is using, for a controller to give its
// servers. Test processes run at the same time, and two that picked ports at random could pick the same ones: a server that
// cannot bind its port ends, and the one that holds it answers for it. The block is claimed with a lock on a file, which ends with
// the process.
func PortsFrom() int {
	claim.Do(func() {
		dir := filepath.Join(os.TempDir(), "gusto-test-ports")
		for block := range blocks {
			lock, err := filelock.Try(filepath.Join(dir, fmt.Sprint(block)))
			if err == nil {
				held, from = lock, firstPort+block*blockSize
				return
			}
		}
		panic("every block of test ports is claimed by a test process that is running")
	})
	return from
}
