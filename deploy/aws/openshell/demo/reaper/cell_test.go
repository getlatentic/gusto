package reaper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// onACell runs the script as a cell's host with the clock at 100000, the front door's beat at the time
// given (a negative one for no beat file), and the host up for so long.
func onACell(t *testing.T, beatAt, uptime int) (poweredOff bool, output string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	log := filepath.Join(dir, "systemctl.log")
	os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\necho \"$@\" >> "+log+"\n"), 0o755)
	conf := filepath.Join(dir, "demo.conf")
	os.WriteFile(conf, []byte("MAX_HOURS=4\nROLE=cell\n"), 0o644)
	beat := filepath.Join(dir, "orchestrator.beat")
	if beatAt >= 0 {
		os.WriteFile(beat, []byte("x\n"), 0o644)
		os.Chtimes(beat, timeAt(beatAt), timeAt(beatAt))
	}
	command := exec.Command("bash", script)
	command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HAAS_CONF="+conf, "HAAS_BEAT_FILE="+beat, "HAAS_LEASE_FILE="+filepath.Join(dir, "no-lease.json"),
		"HAAS_NOW=100000", fmt.Sprintf("HAAS_UPTIME=%d", uptime))
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	text, _ := os.ReadFile(log)
	return strings.TrimSpace(string(text)) == "poweroff", string(out)
}

func TestACellWorksWhileItsFrontDoorIsHeardAndPowersOffWhenItIsNot(t *testing.T) {
	for name, tc := range map[string]struct {
		beatAt, uptime int
		off            bool
	}{
		"heard a moment ago":                     {99990, 3000, false},
		"heard just inside the limit":            {100000 - 299, 3000, false},
		"silent for exactly the limit":           {100000 - 300, 3000, true},
		"silent for long":                        {1000, 3000, true},
		"never heard, and just started":          {-1, 60, false},
		"never heard, and up past the grace":     {-1, 900, true},
		"silent, but the front door may be late": {1000, 899, false},
		"heard, but up as long as any host may":  {99990, 4 * 3600, true},
	} {
		t.Run(name, func(t *testing.T) {
			off, out := onACell(t, tc.beatAt, tc.uptime)
			if off != tc.off {
				t.Errorf("powered off: %v, wanted %v: %s", off, tc.off, out)
			}
			if tc.off && !strings.Contains(out, "powering off") {
				t.Errorf("it says nothing of why: %q", out)
			}
		})
	}
}

func TestACellHasNoLeaseSoAStrayOneDoesNotEndIt(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	log := filepath.Join(dir, "systemctl.log")
	os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\necho \"$@\" >> "+log+"\n"), 0o755)
	conf := filepath.Join(dir, "demo.conf")
	os.WriteFile(conf, []byte("MAX_HOURS=4\nROLE=cell\n"), 0o644)
	lease := filepath.Join(dir, "lease.json")
	os.WriteFile(lease, []byte(`{"deadline": 1, "hardMax": 2}`), 0o644)
	beat := filepath.Join(dir, "orchestrator.beat")
	os.WriteFile(beat, []byte("x\n"), 0o644)
	os.Chtimes(beat, timeAt(99999), timeAt(99999))
	command := exec.Command("bash", script)
	command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HAAS_CONF="+conf, "HAAS_BEAT_FILE="+beat, "HAAS_LEASE_FILE="+lease, "HAAS_NOW=100000", "HAAS_UPTIME=3000")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if text, _ := os.ReadFile(log); len(text) != 0 {
		t.Errorf("a lease is not a cell's: %s", text)
	}
}

func timeAt(seconds int) time.Time { return time.Unix(int64(seconds), 0) }
