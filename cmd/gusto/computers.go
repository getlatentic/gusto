package main

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/quota"
	"github.com/getlatentic/gusto/pkg/job"
)

// computersOf is the record of the computers people in this organization have, when this server is an organization's and runs
// tasks in sandboxes: dir is its directory, org the organization, and the file quota the limit on how many.
func computersOf(dir, org, quotaFile string) (*computer.Registry, error) {
	if dir == "" {
		return nil, nil
	}
	if org == "" {
		return nil, errors.New("GUSTO_COMPUTERS names the computers of an organization, and GUSTO_ORG says which")
	}
	registry, err := computer.Open(dir, org)
	if err != nil {
		return nil, err
	}
	if quotaFile != "" {
		registry.UseQuota(quota.Open(quotaFile))
	}
	return registry, nil
}

// personWhoAsked is the address of the person a task was asked for by, or nothing when
// it was asked by a token or a script, which have no computer of their own.
func personWhoAsked(index *jobs.Index, dir job.Dir) string {
	record, found := index.Get(filepath.Base(dir.Path()))
	if !found {
		return ""
	}
	return aPerson(record.Asked)
}

// personWhoPays is the person whose computer runs a task: a room's agent's owner, else the person who asked.
func personWhoPays(index *jobs.Index, dir job.Dir) string {
	record, found := index.Get(filepath.Base(dir.Path()))
	if !found {
		return ""
	}
	return aPerson(record.Payer())
}

// aPerson is an address as a person's, or nothing for a token or a script.
func aPerson(who string) string {
	if !strings.Contains(who, "@") || strings.HasPrefix(who, "token:") {
		return ""
	}
	return strings.ToLower(who)
}
