package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getlatentic/haas/internal/integration"
	"github.com/getlatentic/haas/internal/job"
)

// Integrations is the integrations an organisation has imported, which a setup may name. Only the versions an administrator has
// turned on are given to a job.
type Integrations interface {
	Get(name string) (*integration.Integration, error)
	Active() ([]integration.Active, error)
}

// UseIntegrations lets setups name integrations, and is called before the store is used. Without it a setup that names one is refused.
func (s *Store) UseIntegrations(from Integrations) {
	s.guard.Lock()
	defer s.guard.Unlock()
	s.integrations = from
}

// Use is what one integration a setup names gives it, as the setup is now.
type Use struct {
	Name string
	// Skills and Servers are what a job would be given.
	Skills  int
	Servers int
	// Held back are the servers that cannot be used yet, and why.
	Held []string
	// On says the integration is turned on. A setup that names one that is off cannot start a job.
	On bool
}

var unsafeName = regexp.MustCompile(`[^a-z0-9-]+`)

// serverName is what a job calls a server of an integration: the integration's name and the server's, so that two integrations
// that each have a server called api do not meet. It is a name the harnesses take.
func serverName(plugin, server string) string {
	name := unsafeName.ReplaceAllString(strings.ToLower(plugin+"-"+server), "-")
	name = strings.Trim(name, "-")
	if len(name) > 64 {
		name = strings.TrimRight(name[:64], "-")
	}
	return name
}

// usable is the server as a job can call it now. Only a streamable HTTP server is: a program that an integration brings would run
// inside the job's box, which is what integrations are for not doing, and waits for the gateway, and the older SSE transport is
// not one the harnesses here take.
func usable(plugin string, server integration.Server) (job.Server, string) {
	switch server.Transport {
	case integration.StreamableHTTP:
		return job.Server{Name: serverName(plugin, server.Name), URL: server.URL, Headers: server.Headers}, ""
	case integration.Stdio:
		return job.Server{}, server.Name + " runs a program, which waits for the gateway"
	}
	return job.Server{}, server.Name + " uses the older SSE transport"
}

// uses is what each integration a setup names gives it now.
func (s *Store) uses(held Setup) []Use {
	if s.integrations == nil || len(held.Integrations) == 0 {
		return nil
	}
	var all []Use
	for _, name := range held.Integrations {
		use := Use{Name: name}
		if found, err := s.integrations.Get(name); err == nil {
			if version, on := found.Version(found.On); on {
				use.On, use.Skills = true, len(version.Plugin.Skills)
				for _, server := range version.Plugin.Servers {
					if _, why := usable(name, server); why != "" {
						use.Held = append(use.Held, why)
					} else {
						use.Servers++
					}
				}
			}
		}
		all = append(all, use)
	}
	return all
}

// checkIntegrations says why the names a setup holds cannot be kept, or nothing.
func (s *Store) checkIntegrations(names []string) error {
	if len(names) == 0 {
		return nil
	}
	if s.integrations == nil {
		return fmt.Errorf("this deployment keeps no integrations")
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			return fmt.Errorf("the integration %s is named twice", name)
		}
		seen[name] = true
		if _, err := s.integrations.Get(name); err != nil {
			return fmt.Errorf("no integration called %s has been imported", name)
		}
	}
	return nil
}

// carryIntegrations puts the skills and servers of the integrations a setup names into a job's directory, for the version that is
// on. An integration that is off, or gone, is a job that does not start, since a job that quietly lacked what its setup names
// would run as a bare harness under the setup's name.
func (s *Store) carryIntegrations(held Setup, dir job.Dir, carried *job.Setup, servers *[]job.Server) error {
	if len(held.Integrations) == 0 {
		return nil
	}
	if s.integrations == nil {
		return fmt.Errorf("the setup %s names integrations, and this deployment keeps none", held.Name)
	}
	active, err := s.integrations.Active()
	if err != nil {
		return err
	}
	on := map[string]integration.Active{}
	for _, one := range active {
		on[one.Name] = one
	}
	have := map[string]bool{}
	for _, name := range carried.Skills {
		have[name] = true
	}
	taken := map[string]bool{}
	for _, name := range carried.Servers {
		taken[name] = true
	}
	for _, name := range held.Integrations {
		one, there := on[name]
		if !there {
			return fmt.Errorf("the setup %s names the integration %s, which is off: an administrator turns it on", held.Name, name)
		}
		for _, skill := range one.Version.Plugin.Skills {
			if have[skill.Name] {
				return fmt.Errorf("the setup %s gets a skill called %s more than once, and from %s among them", held.Name, skill.Name, name)
			}
			have[skill.Name] = true
			from := filepath.Join(one.Dir, filepath.FromSlash(skill.Dir))
			to := filepath.Join(dir.SetupDir(), "skills", skill.Name)
			if err := os.CopyFS(to, os.DirFS(from)); err != nil {
				return err
			}
			carried.Skills = append(carried.Skills, skill.Name)
		}
		for _, server := range one.Version.Plugin.Servers {
			definition, why := usable(name, server)
			if why != "" {
				continue
			}
			if taken[definition.Name] {
				return fmt.Errorf("the setup %s gets an MCP server called %s more than once", held.Name, definition.Name)
			}
			taken[definition.Name] = true
			*servers = append(*servers, definition)
			carried.Servers = append(carried.Servers, definition.Name)
		}
		carried.Integrations = append(carried.Integrations, job.IntegrationUse{
			Name: name, Version: one.Version.ID, Label: one.Version.Label, Commit: one.Version.Source.Commit})
	}
	return nil
}
