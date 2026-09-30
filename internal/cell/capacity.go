package cell

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Capacity is what a cell has left to give an organisation's computers.
type Capacity struct {
	CPUs               int `json:"cpus"`
	MemoryAvailableMiB int `json:"memoryAvailableMiB"`
}

// Measure reads the machine's own capacity. Where the system does not say, it is a machine
// with a lot of memory, so that a development machine can be a cell.
func Measure() Capacity {
	available := 16384
	if file, err := os.Open("/proc/meminfo"); err == nil {
		defer file.Close()
		scan := bufio.NewScanner(file)
		for scan.Scan() {
			if rest, found := strings.CutPrefix(scan.Text(), "MemAvailable:"); found {
				if kib, err := strconv.Atoi(strings.Fields(rest)[0]); err == nil {
					available = kib / 1024
				}
			}
		}
	}
	return Capacity{CPUs: runtime.NumCPU(), MemoryAvailableMiB: available}
}
