package main

import (
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/apitoken"
	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/integration"
	"github.com/getlatentic/gusto/internal/preset"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/internal/signing"
)

// useSetups is what a task is set up with: the setups kept beside the instances and the operator's MCP servers they choose
// from, one file each in GUSTO_MCP; the integrations an administrator imported, of which a task gets the version that is on; the
// presets and the crew of agents; and where sign-ins are kept, beside the sessions, so a deploy does not sign people out.
func (s *server) useSetups() error {
	setups, err := setup.Open(setting("GUSTO_SETUPS", "setups"))
	if err != nil {
		return err
	}
	servers, err := setup.OpenServers(os.Getenv("GUSTO_MCP"))
	if err != nil {
		return err
	}
	integrations, err := integration.Open(setting("GUSTO_INTEGRATIONS", "integrations"))
	if err != nil {
		return err
	}
	setups.UseIntegrations(integrations)
	s.pages.UseIntegrations(integrations, integration.Clone{AllowPrivate: os.Getenv("GUSTO_INTEGRATIONS_PRIVATE") == "1"})
	s.pages.UseSetups(setups, servers)
	presets, err := preset.Open(setting("GUSTO_PRESETS", "presets"))
	if err != nil {
		return err
	}
	s.pages.UsePresets(presets)
	agents, err := crew.Open(setting("GUSTO_CREW", "crew"))
	if err != nil {
		return err
	}
	s.pages.UseCrew(agents)
	s.pages.UseDesktop(os.Getenv("GUSTO_DESKTOP"))
	keeper := setting("GUSTO_KEEPER", filepath.Join(setting("GUSTO_SESSIONS", "sessions"), ".keeper.json"))
	if err := s.pages.UseSessionFile(keeper); err != nil {
		return err
	}
	s.pages.UseInBuilding(s.way.InBuilding())
	return nil
}

// useHistory is what the pages show of what happened: the comparisons, the signers a task's record is checked against, the way
// verify, show, pr open and export check them, the changes to the platform, in a file only this server appends to, and the
// API tokens.
func (s *server) useHistory() error {
	lineups, err := compare.OpenBoard(setting("GUSTO_COMPARISONS", "comparisons"))
	if err != nil {
		return err
	}
	s.pages.UseBoard(lineups)
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return err
	}
	s.pages.UseKeyring(keyring)
	changes, err := openChanges(keyring)
	if err != nil {
		return err
	}
	s.started(func() { changes.Close() })
	s.pages.UseChanges(changes)
	if path := os.Getenv("GUSTO_API_TOKENS"); path != "" {
		tokens, err := apitoken.Open(path)
		if err != nil {
			return err
		}
		s.pages.UseTokens(tokens)
	}
	return nil
}
