package main

import (
	"path/filepath"

	"github.com/getlatentic/gusto/internal/apitoken"
	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/integration"
	"github.com/getlatentic/gusto/internal/preset"
	"github.com/getlatentic/gusto/internal/setup"
)

// offerSetups is what a task is set up with: the setups kept beside the instances and the operator's MCP servers they choose
// from, one file each in GUSTO_MCP; the integrations an administrator imported, of which a task gets the version that is on; the
// presets and the crew of agents; and where sign-ins are kept, beside the sessions, so a deploy does not sign people out.
func (s *server) offerSetups() error {
	stores, served := s.cfg.stores, s.cfg.served
	setups, err := setup.Open(stores.setups)
	if err != nil {
		return err
	}
	servers, err := setup.OpenServers(served.mcp)
	if err != nil {
		return err
	}
	integrations, err := integration.Open(stores.integrations)
	if err != nil {
		return err
	}
	setups.UseIntegrations(integrations)
	s.pages.UseIntegrations(integrations, integration.Clone{AllowPrivate: served.integrationsPrivate})
	s.pages.UseSetups(setups, servers)
	presets, err := preset.Open(stores.presets)
	if err != nil {
		return err
	}
	s.pages.UsePresets(presets)
	agents, err := crew.Open(stores.crew)
	if err != nil {
		return err
	}
	s.pages.UseCrew(agents)
	s.pages.UseDesktop(served.desktop)
	keeper := firstOf(served.keeper, filepath.Join(stores.sessions, ".keeper.json"))
	if err := s.pages.UseSessionFile(keeper); err != nil {
		return err
	}
	s.pages.UseInBuilding(s.way.InBuilding())
	return nil
}

// offerHistory is what the pages show of what happened: the comparisons, the signers a task's record is checked against, the way
// verify, show, pr open and export check them, the changes to the platform, in a file only this server appends to, and the
// API tokens.
func (s *server) offerHistory() error {
	lineups, err := compare.OpenBoard(s.cfg.stores.comparisons)
	if err != nil {
		return err
	}
	s.pages.UseBoard(lineups)
	keyring, err := s.cfg.sign.openKeyring()
	if err != nil {
		return err
	}
	s.pages.UseKeyring(keyring)
	changes, err := changelog.Open(s.cfg.stores.changes, s.signs.Key, keyring)
	if err != nil {
		return err
	}
	s.started(func() { changes.Close() })
	s.pages.UseChanges(changes)
	if path := s.cfg.stores.apiTokens; path != "" {
		tokens, err := apitoken.Open(path)
		if err != nil {
			return err
		}
		s.pages.UseTokens(tokens)
	}
	return nil
}
