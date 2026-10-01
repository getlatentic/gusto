package cloud

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DemoLimits are the limits on how many demos start, which the owner of a front door changes while it runs. Zero is no
// limit, except for MaxOrgs, the most demos at once, which is at least one.
type DemoLimits struct {
	PerDay     int `json:"perDay"`
	PerPerson  int `json:"perPerson"`
	PerCompany int `json:"perCompany"`
	MaxOrgs    int `json:"maxOrgs"`
}

const (
	mostPerLimit = 1000
	mostAtOnce   = 50
)

func (s *Store) limitsFile() string { return filepath.Join(s.root, "demo-limits.json") }

// DemoLimits are the limits now: what the owner last set, or what the front door was started with when they have set
// none. They are read whenever a demo is asked for, so a change holds from the next one without a restart.
func (s *Store) DemoLimits() DemoLimits {
	set := s.set.Demo
	started := DemoLimits{PerDay: set.PerDay, PerPerson: set.PerPerson, PerCompany: set.PerCompany, MaxOrgs: max(set.MaxOrgs, 1)}
	var kept DemoLimits
	if readFile(s.limitsFile(), &kept) != nil {
		return started
	}
	kept.MaxOrgs = max(kept.MaxOrgs, 1)
	return kept
}

// DemoLimitsSet says the owner has set the limits, so that they are not the ones the front door started with.
func (s *Store) DemoLimitsSet() bool {
	_, err := os.Stat(s.limitsFile())
	return err == nil
}

// SetDemoLimits keeps the limits the owner chose.
func (s *Store) SetDemoLimits(limits DemoLimits) error {
	for name, value := range map[string]int{"a day": limits.PerDay, "a person": limits.PerPerson, "a company": limits.PerCompany} {
		if value < 0 || value > mostPerLimit {
			return fmt.Errorf("the most demos %s is from 0 to %d", name, mostPerLimit)
		}
	}
	if limits.MaxOrgs < 1 || limits.MaxOrgs > mostAtOnce {
		return fmt.Errorf("the most demos at once is from 1 to %d", mostAtOnce)
	}
	return writeFile(s.limitsFile(), limits, 0o600)
}

// ResetDemoLimits goes back to the limits the front door was started with.
func (s *Store) ResetDemoLimits() error {
	if err := os.Remove(s.limitsFile()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
