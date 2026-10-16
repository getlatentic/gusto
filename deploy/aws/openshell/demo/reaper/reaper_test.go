package reaper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const script = "../../ansible/roles/demo/files/haas-lease-reaper"

// run is the script on a host with the lease given ("" for none), a clock at 1000, and the
// time it has been up. It says whether the host was told to power off.
func run(t *testing.T, lease string, uptime int) (poweredOff bool, output string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	log := filepath.Join(dir, "systemctl.log")
	os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\necho \"$@\" >> "+log+"\n"), 0o755)
	conf := filepath.Join(dir, "demo.conf")
	os.WriteFile(conf, []byte("MAX_HOURS=4\n"), 0o644)
	leaseFile := filepath.Join(dir, "lease.json")
	if lease != "" {
		os.WriteFile(leaseFile, []byte(lease), 0o644)
	}
	command := exec.Command("bash", script)
	command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GUSTO_CONF="+conf, "GUSTO_LEASE_FILE="+leaseFile, "GUSTO_NOW=1000", fmt.Sprintf("GUSTO_UPTIME=%d", uptime))
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	text, _ := os.ReadFile(log)
	return strings.TrimSpace(string(text)) == "poweroff", string(out)
}

func TestAHostIsLeftAloneWhileItsLeaseHasTimeAndPoweredOffWhenItHasNone(t *testing.T) {
	for name, tc := range map[string]struct {
		lease  string
		uptime int
		off    bool
	}{
		"time left":                        {`{"deadline": 1001, "hardMax": 99999}`, 600, false},
		"the second it ends":               {`{"deadline": 1000, "hardMax": 99999}`, 600, true},
		"long ended":                       {`{"deadline": 10, "hardMax": 99999}`, 600, true},
		"no lease yet, and just started":   {"", 30, false},
		"a file that is not a lease":       {"not json", 600, true},
		"a lease with no deadline":         {`{"hardMax": 5}`, 600, true},
		"time left but up as long as ever": {`{"deadline": 999999, "hardMax": 999999}`, 4 * 3600, true},
		"no lease, and up as long as ever": {"", 4*3600 + 1, true},
		"time left and up just under":      {`{"deadline": 5000, "hardMax": 999999}`, 4*3600 - 1, false},
	} {
		t.Run(name, func(t *testing.T) {
			off, out := run(t, tc.lease, tc.uptime)
			if off != tc.off {
				t.Errorf("powered off: %v, wanted %v: %s", off, tc.off, out)
			}
			if tc.off && !strings.Contains(out, "powering off") {
				t.Errorf("it says nothing of why: %q", out)
			}
		})
	}
}
