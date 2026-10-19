package registry

import (
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"sort"
	"strings"
)

// maxAccess is the most people an organization may limit, which keeps the file a size a page can show.
const maxAccess = 500

// Access is a person who may use only some of the models, and which.
type Access struct {
	Email  string   `json:"email"`
	Models []string `json:"models"`
}

// AllowedFor is the models a person is limited to, and whether they are limited at all. A person nobody has said
// anything about may use every model the organization has.
func (r *Registry) AllowedFor(email string) ([]string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, limit := range r.Document().Access {
		if limit.Email == email {
			return slices.Clone(limit.Models), true
		}
	}
	return nil, false
}

// Limits are the people who are limited, in order of address.
func (r *Registry) Limits() []Access {
	limits := slices.Clone(r.Document().Access)
	sort.Slice(limits, func(i, j int) bool { return limits[i].Email < limits[j].Email })
	return limits
}

// SetAccess limits a person to these models, which must be ones the organization has now. No models removes the limit.
func (r *Registry) SetAccess(email string, models []string) error {
	address, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || address.Name != "" || len(address.Address) > 254 {
		return errors.New("that is not an email address")
	}
	email = strings.ToLower(address.Address)
	have := r.Effective().Models
	for _, model := range models {
		if !slices.Contains(have, model) {
			return fmt.Errorf("%s is not a model this organization has", model)
		}
	}
	models = slices.Clone(models)
	sort.Strings(models)
	models = slices.Compact(models)
	return r.change(func(d *Document) error {
		i := slices.IndexFunc(d.Access, func(a Access) bool { return a.Email == email })
		switch {
		case len(models) == 0 && i < 0:
			return ErrNoSuch
		case len(models) == 0:
			d.Access = slices.Delete(d.Access, i, i+1)
		case i >= 0:
			d.Access[i].Models = models
		case len(d.Access) >= maxAccess:
			return errors.New("that is as many people as can be limited")
		default:
			d.Access = append(d.Access, Access{Email: email, Models: models})
		}
		return nil
	})
}
