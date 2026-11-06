package main

import (
	"os"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/settings"
)

// serveSettings is every setting gusto serve reads, read once when it starts.
type serveSettings struct {
	core   coreSettings
	stores storeSettings
	served servedSettings
	access accessSettings
	way    wayOutSettings
	boxes  sandboxSettings
	news   newsSettings
}

func (s *serveSettings) declare(set *settings.Set) {
	s.core.declare(set)
	s.stores.declare(set)
	s.served.declare(set)
	s.access.declare(set)
	s.way.declare(set)
	s.boxes.declare(set)
	s.news.declare(set)
}

// loadServe reads serve's settings from this process's environment. The set is kept, so the settings people sign in under can
// be recorded as they were set.
func loadServe() (serveSettings, *settings.Set, error) {
	var s serveSettings
	set := settings.New()
	s.declare(set)
	return s, set, set.Load(os.Getenv)
}

// loadSettings reads the settings that declare declares from this process's environment, for a command that needs only some.
func loadSettings(declare ...func(*settings.Set)) error {
	set := settings.New()
	for _, each := range declare {
		each(set)
	}
	return set.Load(os.Getenv)
}

// paramsOf is the settings that declare declares as a command's help lists them.
func paramsOf(declare func(*settings.Set)) []cmdline.Param {
	set := settings.New()
	declare(set)
	var params []cmdline.Param
	for _, each := range set.Params() {
		params = append(params, cmdline.Param{Name: each.Name, Text: each.Text})
	}
	return params
}

// coreSettings is where the server listens, who it is for and how a person signs in with the shared password.
type coreSettings struct {
	addr        string
	password    string
	behindProxy bool
	local       bool
	org         string
	publicURL   string
}

func (c *coreSettings) declare(set *settings.Set) {
	set.Secret(&c.password, "GUSTO_PASSWORD", "the server's shared sign-in; required")
	set.String(&c.addr, "GUSTO_ADDR", "127.0.0.1:8040", "where to listen")
	set.Bool(&c.behindProxy, "GUSTO_BEHIND_PROXY", "trust the address a proxy in front says a request came from")
	set.Bool(&c.local, "GUSTO_LOCAL", "a server for one person on their own computer, listening on the loopback only")
	set.String(&c.org, "GUSTO_ORG", "", "the organization this server is, as its gusto cloud names it")
	set.String(&c.publicURL, "GUSTO_PUBLIC_URL", "", "this server's address as people reach it, which links and sign-ins come back to")
}

// servedSettings is where the server keeps what its pages serve, beside the stores every command shares.
type servedSettings struct {
	apps        string
	mcp         string
	keeper      string
	members     string
	inbox       string
	orgSettings string
	quota       string
	desktop     string
	agents      string
	// integrationsPrivate lets an integration be imported from a private address.
	integrationsPrivate bool
}

func (s *servedSettings) declare(set *settings.Set) {
	set.String(&s.apps, "GUSTO_APPS", "apps", "the directory the web apps agents built are kept in")
	set.String(&s.mcp, "GUSTO_MCP", "", "the directory of the operator's MCP servers, one file each")
	set.String(&s.keeper, "GUSTO_KEEPER", "", "the file sign-ins are kept in, so a deploy does not sign people out (default .keeper.json in GUSTO_SESSIONS)")
	set.String(&s.members, "GUSTO_MEMBERS", "", "the file of the organization's members and their roles")
	set.String(&s.inbox, "GUSTO_INBOX", "", "the directory gusto cloud leaves messages for this server in")
	set.String(&s.orgSettings, "GUSTO_ORG_SETTINGS", "", "the file of what the organization's admins chose")
	set.String(&s.quota, "GUSTO_QUOTA", "", "the file of what the organization may run, which its gusto cloud writes")
	set.String(&s.desktop, "GUSTO_DESKTOP", "", "the directory the desktop app's downloads are served from")
	set.String(&s.agents, "GUSTO_AGENTS", "agents", "the directory gusto installs agents into on a person's own computer")
	set.Bool(&s.integrationsPrivate, "GUSTO_INTEGRATIONS_PRIVATE", "let an integration be imported from a private address")
}

// accessSettings is who may sign in, and how: the identity provider, the allowed people and groups, and the front door of
// the organization's gusto cloud.
type accessSettings struct {
	issuer, clientID, clientSecret, redirect, groupsClaim string
	scopes                                                []string
	allowed, runnerGroups, adminGroups, admins            []string
	foundKeys, localRepos                                 []string
	frontDoor, entryKey, orgName                          string
}

func (a *accessSettings) declare(set *settings.Set) {
	set.String(&a.issuer, "GUSTO_OIDC_ISSUER", "", "the identity provider people sign in with")
	set.String(&a.clientID, "GUSTO_OIDC_CLIENT_ID", "", "this server's client at the identity provider")
	set.Secret(&a.clientSecret, "GUSTO_OIDC_CLIENT_SECRET", "the client's secret at the identity provider")
	set.String(&a.redirect, "GUSTO_OIDC_REDIRECT", "", "the address the identity provider sends a person back to")
	set.String(&a.groupsClaim, "GUSTO_OIDC_GROUPS_CLAIM", "", "the claim a person's groups are in")
	set.List(&a.scopes, "GUSTO_OIDC_SCOPES", "the scopes asked for at sign-in, comma-separated")
	set.List(&a.allowed, "GUSTO_ALLOWED", "the people and domains who may sign in, comma-separated")
	set.List(&a.runnerGroups, "GUSTO_RUNNER_GROUPS", "the groups whose members may run tasks")
	set.List(&a.adminGroups, "GUSTO_ADMIN_GROUPS", "the groups whose members are admins")
	set.List(&a.admins, "GUSTO_ADMINS", "the people who are admins, comma-separated")
	set.List(&a.foundKeys, "GUSTO_FOUND_KEYS", "the model keys found on a person's own computer")
	set.List(&a.localRepos, "GUSTO_LOCAL_REPOS", "the repositories on a person's own computer a task may start from")
	set.String(&a.frontDoor, "GUSTO_FRONT_DOOR", "", "the address of the gusto cloud that sends people here")
	set.String(&a.entryKey, "GUSTO_ENTRY_KEY", "", "the file of the key gusto cloud signs a person's entry with")
	set.String(&a.orgName, "GUSTO_ORG_NAME", "", "what the organization is called, which its pages show")
}

// newsSettings is what the server says and to whom: routine results by webhook and mail, and what its pages show of it.
type newsSettings struct {
	webhook, webhookSecret     string
	mail, mailToken, outbox    string
	chatDefault, deployResult  string
	retentionMinutes, liveConn int
}

func (n *newsSettings) declare(set *settings.Set) {
	set.String(&n.webhook, "GUSTO_NOTIFY_WEBHOOK", "", "the address told how each run of a routine ended, by a signed request")
	set.Secret(&n.webhookSecret, "GUSTO_NOTIFY_SECRET", "the secret the webhook's requests are signed with")
	set.String(&n.mail, "GUSTO_MAIL", "", "how mail is sent to the person who added a routine, as GUSTO_CLOUD_MAIL is")
	set.Secret(&n.mailToken, "GUSTO_MAIL_TOKEN", "the token mail is sent with")
	set.String(&n.outbox, "GUSTO_MAIL_OUTBOX", "outbox", "the directory mail is written to when it is not sent")
	set.String(&n.chatDefault, "GUSTO_CHAT_DEFAULT", "", "what a new chat runs on, as agent=model")
	set.String(&n.deployResult, "GUSTO_DEPLOY_RESULT", "", "how the last deploy went, which the health page shows")
	set.Int(&n.retentionMinutes, "GUSTO_RETENTION_MINUTES", 60, 1, "how often retention removes what its policy says, in minutes")
	set.Int(&n.liveConn, "GUSTO_LIVE_CONNECTIONS", 10000, 1, "the most rooms' pages followed live at once")
}
