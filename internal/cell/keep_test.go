package cell

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTheKeptHostIsStoppedWhenIdleAndStartedAgainRatherThanANewOne(t *testing.T) {
	r := scaling(t)
	r.scaler.Keep = 1
	r.waiting = 1
	r.tick(0)
	if r.hosts.launched != 1 || !r.hosts.hosts[0].Kept {
		t.Fatalf("the first host is not the kept one: %+v", r.hosts.hosts)
	}
	r.cell("i-0aaa1111", false)
	r.waiting = 0
	r.tick(time.Minute)
	r.tick(11 * time.Minute)
	if len(r.hosts.terminated) != 0 || len(r.hosts.stopped) != 1 || r.hosts.stopped[0].ID != "i-0aaa1111" {
		t.Fatalf("an idle kept host is stopped and not removed: terminated %v, stopped %+v", r.hosts.terminated, r.hosts.stopped)
	}
	r.waiting = 1
	r.tick(time.Hour)
	if r.hosts.started != 1 || r.hosts.launched != 1 {
		t.Errorf("the kept host is started rather than a new one made: started %d, launched %d", r.hosts.started, r.hosts.launched)
	}
}

func TestAHostBesideTheKeptOneIsRemovedWhenIdle(t *testing.T) {
	r := scaling(t)
	r.scaler.Keep = 1
	r.hosts.hosts = []Instance{{ID: "i-0aaa1111", Launched: r.now, Kept: true}}
	r.cell("i-0aaa1111", true)
	r.waiting = 1
	r.hosts.next = "i-0bbb2222"
	r.tick(time.Minute)
	r.tick(time.Minute)
	if r.hosts.launched != 1 || r.hosts.hosts[1].Kept {
		t.Fatalf("a second host is made, and is not kept, while one is: %+v", r.hosts.hosts)
	}
	r.cell("i-0bbb2222", false)
	r.waiting = 0
	r.tick(time.Minute)
	r.tick(11 * time.Minute)
	if len(r.hosts.terminated) != 1 || r.hosts.terminated[0] != "i-0bbb2222" || len(r.hosts.stopped) != 0 {
		t.Errorf("the host that is not kept is removed and the busy kept one left: terminated %v, stopped %+v", r.hosts.terminated, r.hosts.stopped)
	}
}

func TestAKeptHostStillStoppingIsNotStartedAndNoSecondIsKept(t *testing.T) {
	r := scaling(t)
	r.scaler.Keep = 1
	r.hosts.stopped = []Instance{{ID: "i-0ccc3333", Kept: true, Stopped: false}}
	r.waiting = 1
	r.tick(0)
	if r.hosts.started != 0 || r.hosts.launched != 1 || r.hosts.hosts[0].Kept {
		t.Errorf("a new host that is not kept is made while the kept one stops: started %d, %+v", r.hosts.started, r.hosts.hosts)
	}
}

func TestAKeptHostThatNeverBecomesACellIsRemoved(t *testing.T) {
	r := scaling(t)
	r.scaler.Keep = 1
	r.waiting = 1
	r.tick(0)
	r.tick(11 * time.Minute)
	if len(r.hosts.terminated) != 1 || len(r.hosts.stopped) != 0 {
		t.Errorf("a kept host that failed to start is removed, not kept: terminated %v, stopped %+v", r.hosts.terminated, r.hosts.stopped)
	}
}

func TestAKeptHostIsTaggedStartedWithTodaysScriptEncodedAndStopped(t *testing.T) {
	script := filepath.Join(t.TempDir(), "user-data.sh")
	os.WriteFile(script, []byte("KIT=@KIT@\n"), 0o644)
	var ran []string
	var value string
	e := &EC2{Template: "cell", UserData: script, Settings: map[string]string{"KIT": "abc"},
		Run: func(args ...string) ([]byte, error) {
			ran = append(ran, strings.Join(args, " "))
			for i, arg := range args {
				if arg == "--value" {
					text, _ := os.ReadFile(strings.TrimPrefix(args[i+1], "file://"))
					value = string(text)
				}
			}
			switch args[1] {
			case "describe-instances":
				if args[2] == "--instance-ids" {
					return []byte("vol-0aaa1111\n"), nil
				}
				return []byte("i-0aaa1111\t2026-10-01T09:00:00+00:00\tstopped\ton\ni-0bbb2222\t2026-10-01T09:05:00+00:00\tstopping\tNone\n"), nil
			case "describe-volumes":
				return []byte("500\n"), nil
			case "run-instances":
				return []byte("i-0aaa1111\n"), nil
			}
			return nil, nil
		}}
	if _, err := e.Launch(true); err != nil || !strings.Contains(ran[0], "{Key=kept,Value=on}") || strings.Contains(ran[0], "block-device-mappings") {
		t.Errorf("a kept host is tagged so and is given the template's disk, which a new disk needs to be read quickly: %s %v", ran[0], err)
	}
	if err := e.Start("i-0aaa1111"); err != nil || value != "S0lUPWFiYwo=" || !strings.HasPrefix(ran[2], "ec2 start-instances --instance-ids i-0aaa1111") {
		t.Errorf("the script is not put in encoded before the host starts: %q %v %v", value, ran, err)
	}
	ran = nil
	if err := e.Stop("i-0aaa1111"); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 4 || !strings.HasPrefix(ran[0], "ec2 stop-instances --instance-ids i-0aaa1111") ||
		!strings.HasPrefix(ran[3], "ec2 modify-volume --volume-id vol-0aaa1111 --iops 3000 --throughput 125") {
		t.Errorf("a stop stops the host and then lowers its disk to the baseline: %v", ran)
	}
	hosts, err := e.Stopped()
	if err != nil || len(hosts) != 2 || !hosts[0].Kept || !hosts[0].Stopped || hosts[1].Kept || hosts[1].Stopped {
		t.Errorf("%+v %v", hosts, err)
	}
	if !strings.Contains(ran[len(ran)-1], "Values=stopping,stopped") {
		t.Errorf("%s", ran[len(ran)-1])
	}
	for _, bad := range []string{"", "i-0aaa1111 --dry-run"} {
		if e.Start(bad) == nil || e.Stop(bad) == nil {
			t.Errorf("%q was taken for a host", bad)
		}
	}
}

func TestADiskAlreadyAtTheBaselineIsLeftAloneAndADiskThatCannotBeLoweredDoesNotFailTheStop(t *testing.T) {
	var ran []string
	throughput, modify := "125", error(nil)
	e := &EC2{Run: func(args ...string) ([]byte, error) {
		ran = append(ran, args[1])
		switch args[1] {
		case "describe-instances":
			return []byte("vol-0aaa1111\n"), nil
		case "describe-volumes":
			return []byte(throughput + "\n"), nil
		case "modify-volume":
			return nil, modify
		}
		return nil, nil
	}}
	if err := e.Stop("i-0aaa1111"); err != nil || slices.Contains(ran, "modify-volume") {
		t.Errorf("a disk at the baseline is not changed again: %v %v", ran, err)
	}
	throughput, modify, ran = "500", errors.New("a disk may be changed once every six hours"), nil
	if err := e.Stop("i-0aaa1111"); err != nil || !slices.Contains(ran, "modify-volume") {
		t.Errorf("a stop that could not lower the disk still stops the host: %v %v", ran, err)
	}
}
