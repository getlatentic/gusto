package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"time"
)

// Provided is haas's own name on a setup it wrote from a template.
const Provided = "haas"

// Provide keeps a setup haas ships up to date with the version it ships. A
// skill still as haas last wrote it takes the new version; a skill a person
// has changed keeps their change. A setup of that name that somebody else made
// is not touched.
func (s *Store) Provide(wanted Setup, offered Servers) error {
	held, found := s.Get(wanted.Name)
	if found && held.Who != Provided {
		return nil
	}
	wanted.Made = time.Now().Unix()
	if found {
		wanted.Made, wanted.Wrote = held.Made, held.Wrote
	}
	wanted.Who = Provided
	if err := s.Save(wanted, offered); err != nil {
		return err
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	if wanted.Wrote == nil {
		wanted.Wrote = map[string]string{}
	}
	for _, skill := range wanted.Skills {
		text := document(skill.Name, strings.Join(strings.Fields(skill.Description), " "), skill.Body)
		if !s.untouched(wanted, skill.Name) {
			continue
		}
		if err := s.put(wanted.Name, skill.Name, text); err != nil {
			return err
		}
		wanted.Wrote[skill.Name] = fingerprint(text)
	}
	return s.saveLocked(wanted)
}

// untouched is a skill nobody but haas has written: absent, matching what
// haas last wrote, or, in a setup saved before haas kept fingerprints, not
// changed since the setup was made.
func (s *Store) untouched(held Setup, skill string) bool {
	path := s.skillFile(held.Name, skill)
	text, err := os.ReadFile(path)
	if err != nil {
		return os.IsNotExist(err)
	}
	if wrote, kept := held.Wrote[skill]; kept {
		return fingerprint(string(text)) == wrote
	}
	info, err := os.Stat(path)
	return err == nil && !info.ModTime().After(time.Unix(held.Made, 0).Add(time.Minute))
}

func fingerprint(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
