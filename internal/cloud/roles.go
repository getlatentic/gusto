package cloud

import "errors"

// ErrBadRole is the refusal of a role that is neither of the two there are.
var ErrBadRole = errors.New("a role is admin or runner")

// SetRole changes the role of a person in the organisation, and says what it was. An
// admin may change anybody's, and the organisation keeps at least one admin, so the
// last cannot be made a runner.
func (s *Store) SetRole(slug string, by Person, address string, role Role) (was Role, err error) {
	if role != Admin && role != Runner {
		return "", ErrBadRole
	}
	email, err := NormalEmail(address)
	if err != nil {
		return "", ErrNoMember
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if by.Org != slug || by.Role != Admin {
		return "", ErrNotAdmin
	}
	target, found, err := s.person(email)
	if err != nil {
		return "", err
	}
	if !found || target.Org != slug {
		return "", ErrNoMember
	}
	if target.Role == role {
		return role, nil
	}
	if target.Role == Admin {
		admins, err := s.admins(slug)
		if err != nil {
			return "", err
		}
		if admins <= 1 {
			return "", ErrLastAdmin
		}
	}
	was, target.Role = target.Role, role
	if err := writeFile(s.personFile(email), target, 0o600); err != nil {
		return "", err
	}
	s.publishRoster(slug)
	return was, nil
}

// admins is how many admins the organisation has.
func (s *Store) admins(slug string) (int, error) {
	members, err := s.members(slug)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, member := range members {
		if member.Role == Admin {
			count++
		}
	}
	return count, nil
}
