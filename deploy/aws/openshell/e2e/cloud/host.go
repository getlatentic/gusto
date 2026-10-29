package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// jobOf is the part of a job's record this run reads from the organization's directory.
type jobOf struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Asked string `json:"asked"`
	Exit  int    `json:"exit"`
}

func (h host) jobsAskedBy(slug, email string) []jobOf {
	names, _ := filepath.Glob(filepath.Join(h.data(slug), "jobs", "*", "state.json"))
	var found []jobOf
	for _, name := range names {
		text, _ := os.ReadFile(name)
		var record jobOf
		if json.Unmarshal(text, &record) == nil && record.Asked == email {
			found = append(found, record)
		}
	}
	return found
}

// host is the machine everything runs on: where the front door keeps what it keeps, the programs that say what the gateway
// holds, what the agents run in, and the isolation check for a container.
type host struct{ root, gusto, gateway, sandbox, probe string }

func (h host) data(slug string) string { return filepath.Join(h.root, "orgs", slug, "data") }

// strangers is the first few files in an organization's directory that belong to someone other than the user this runs as,
// which is the user gusto cloud and its servers run as.
func (h host) strangers(slug string) ([]string, error) {
	out, err := exec.Command("find", filepath.Join(h.root, "orgs", slug), "!", "-uid", strconv.Itoa(os.Getuid()), "-print").Output()
	if err != nil {
		return nil, err
	}
	found := strings.Fields(string(out))
	return found[:min(len(found), 5)], nil
}

// finished waits until every job the person asked for is done, and there are n of them.
func (h host) finished(slug, email string, n int, within time.Duration) ([]jobOf, error) {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		jobs := h.jobsAskedBy(slug, email)
		done := 0
		for _, each := range jobs {
			if each.State == "passed" || each.State == "failed" || each.State == "interrupted" {
				done++
			}
		}
		if len(jobs) >= n && done == len(jobs) {
			return jobs, nil
		}
		time.Sleep(3 * time.Second)
	}
	return h.jobsAskedBy(slug, email), fmt.Errorf("%s: %d jobs were not all done in %s", email, n, within)
}

func (h host) computers() (string, error) {
	command := exec.Command(h.gusto, "cloud", "workspaces")
	command.Env = append(os.Environ(), "GUSTO_CLOUD_ROOT="+h.root)
	out, err := command.CombinedOutput()
	return string(out), err
}

// sandboxes is the names of what the gateway holds.
func (h host) sandboxes() ([]string, error) {
	out, err := exec.Command("openshell", "--gateway", h.gateway, "sandbox", "list").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, out)
	}
	var names []string
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if fields := strings.Fields(line); i > 0 && len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names, nil
}

// fileIn is a file of an instance's workspace, wherever the organization keeps instances.
func (h host) fileIn(slug, instance, name string) (string, error) {
	found, _ := filepath.Glob(filepath.Join(h.data(slug), "*", instance, "workspace", name))
	if len(found) == 0 {
		return "", fmt.Errorf("%s has no %s", instance, name)
	}
	text, err := os.ReadFile(found[0])
	return strings.TrimSpace(string(text)), err
}
