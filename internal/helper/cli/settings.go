package cli

import (
	"fmt"
	"os"

	"github.com/getlatentic/gusto/internal/helper"
	"github.com/getlatentic/gusto/internal/settings"
)

// account is what every command that signs in to a server reads: the server, and where this computer keeps its token.
type account struct {
	server string
	vault  string
}

func (a *account) declare(set *settings.Set) {
	declareServer(set, &a.server)
	set.Choice(&a.vault, "GUSTO_VAULT", "keychain", []string{"keychain", "file"},
		"where this computer keeps its sign-in: the keychain, or a file only you can read")
}

func (a account) openVault() (helper.Vault, error) {
	return helper.OpenVault(os.Stderr, a.vault == "file")
}

func declareServer(set *settings.Set, into *string) {
	set.String(into, "GUSTO_SERVER", "", "your organization's server, when -server is not given")
}

// syncSettings is what gusto-helper sync reads.
type syncSettings struct {
	account
	instance string
}

func (s *syncSettings) declare(set *settings.Set) {
	s.account.declare(set)
	set.String(&s.instance, "GUSTO_INSTANCE", "", "the agent whose folder this is, when -instance is not given")
}

// acpSettings is what gusto-helper acp reads.
type acpSettings struct {
	account
	instance string
	kind     string
}

func (s *acpSettings) declare(set *settings.Set) {
	s.account.declare(set)
	set.String(&s.instance, "GUSTO_INSTANCE", "", "the agent to work with, when -instance is not given")
	set.String(&s.kind, "GUSTO_AGENT_KIND", "", "the kind of agent to start, when -agent is not given")
}

// AccountSettings, ServiceSettings, SyncSettings and ACPSettings declare what each command reads, for its help to list.
func AccountSettings(set *settings.Set) { new(account).declare(set) }

func ServiceSettings(set *settings.Set) { declareServer(set, new(string)) }

func SyncSettings(set *settings.Set) { new(syncSettings).declare(set) }

func ACPSettings(set *settings.Set) { new(acpSettings).declare(set) }

// loaded reads what declare declares from lookup, and says which setting is wrong when one does not read.
func loaded(lookup func(string) string, declare func(*settings.Set)) bool {
	set := settings.New()
	declare(set)
	if err := set.Load(lookup); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}
	return true
}
