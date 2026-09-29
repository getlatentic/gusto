package cloud

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// claim is who owns a company's domain: the organisation whose admin first
// confirmed an address at it.
type claim struct {
	Org string `json:"org"`
}

func (s *Store) claimFile(domain string) string {
	return filepath.Join(s.root, "domains", domain+".json")
}

// DomainOwner is the organisation that has claimed a domain, if one has.
func (s *Store) DomainOwner(domain string) (slug string, claimed bool, err error) {
	var owner claim
	err = readFile(s.claimFile(domain), &owner)
	if missing(err) {
		return "", false, nil
	}
	return owner.Org, err == nil, err
}

// claimDomain gives a domain to an organisation, unless another has it. Creating
// the file is the claim, so two organisations verified together cannot both
// have it. It says whether the organisation has the domain now.
func (s *Store) claimDomain(domain, slug string) (bool, error) {
	err := createFile(s.claimFile(domain), claim{Org: slug}, 0o600)
	if errors.Is(err, fs.ErrExist) {
		owner, _, err := s.DomainOwner(domain)
		return owner == slug, err
	}
	return err == nil, err
}

func (s *Store) makeDomains() error { return os.MkdirAll(filepath.Join(s.root, "domains"), 0o700) }
