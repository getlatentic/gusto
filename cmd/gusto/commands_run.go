package main

import "github.com/getlatentic/gusto/cmd/internal/cmdline"

var envServer = []cmdline.Param{
	{Name: "GUSTO_PASSWORD", Text: "the server's shared sign-in; required"},
	{Name: "GUSTO_ADDR", Text: "where to listen (default 127.0.0.1:8040)"},
	{Name: "GUSTO_JOBS", Text: "the directory tasks are kept in (default jobs)"},
	{Name: "GUSTO_SESSIONS", Text: "the directory agents' conversations are kept in (default sessions)"},
	{Name: "GUSTO_SANDBOX", Text: "how tasks are held: container, openshell, process or here"},
	{Name: "GUSTO_JOB_CONCURRENCY", Text: "how many tasks run at once (default 1)"},
	{Name: "GUSTO_MODELS", Text: "the models the server offers"},
	{Name: "GUSTO_OIDC_ISSUER", Text: "sign people in through this OpenID Connect issuer"},
	{Name: "GUSTO_ALLOWED", Text: "the people who may sign in, separated by commas"},
	{Name: "GUSTO_ADMINS", Text: "the people who administer the server"},
	{Name: "GUSTO_API_TOKENS", Text: "the file of API tokens"},
	{Name: "GUSTO_RETENTION", Text: "the directory of the retention policy and holds"},
	{Name: "GUSTO_REGISTRY", Text: "the directory of the organization's registry of models"},
}

func runCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "serve", Group: groupRun, Summary: "Serve the pages: submit a task, watch it, read the record",
			Description: "Runs the gusto server: the pages for submitting a task, watching it and reading its record, and the " +
				"queue that runs tasks. It will not start without GUSTO_PASSWORD. Many more settings, for sign-in, limits, models and " +
				"storage, are read from GUSTO_ variables; docs/REFERENCE.md lists them.",
			Env:      envServer,
			Examples: []string{"GUSTO_PASSWORD=secret gusto serve"},
			SeeAlso:  []string{"local", "cloud", "doctor"},
			Run:      none("serve", func() int { return serve(nil) }),
		},
		{
			Name: "open", Group: groupRun, Summary: "Open gusto on this computer in the browser, signed in",
			Description: "Opens a new sign-in link to the gusto that runs for you on this computer, and prints it when no browser opens.",
			Env:         []cmdline.Param{{Name: "GUSTO_LOCAL_HOME", Text: "where gusto keeps your work on this computer"}},
			Examples:    []string{"gusto open"},
			SeeAlso:     []string{"local install"},
			Run:         none("open", openCommand),
		},
		{
			Name: "local", Group: groupRun, Summary: "Run gusto for one person on this computer",
			Description: "Gusto for one person on their own computer: it starts when you sign in, listens on the loopback only, and " +
				"keeps its data in your account's data folder.",
			Env: []cmdline.Param{
				{Name: "GUSTO_LOCAL_HOME", Text: "where gusto keeps your work, instead of the system's place for an app's data"},
				{Name: "GUSTO_KEYCHAIN", Text: "off keeps model keys in files only you can read, not in the keychain"},
			},
			SeeAlso: []string{"open", "doctor"},
			Subs:    localCommands(),
		},
		{
			Name: "cloud", Group: groupRun, Summary: "Run gusto cloud, the hosted server for accounts and routing",
			Usage: []string{"", "workspaces", "migrate"},
			Description: "With no subcommand, runs gusto cloud: sign-up, sign-in and the organizations' records, and the servers it " +
				"starts for each organization and sends their people to. It runs no tasks itself. Its settings are the GUSTO_CLOUD_ " +
				"variables; docs/CLOUD.md lists them. The accounts are in a database that several front doors can share: SQLite in " +
				"cloud.db under the root by default, or Postgres when GUSTO_DATABASE_URL names it (docs/CLOUD-DATABASE.md).",
			Env: append([]cmdline.Param{
				{Name: "GUSTO_CLOUD_ADDR", Text: "where to listen (default 127.0.0.1:8050)"},
				{Name: "GUSTO_CLOUD_URL", Text: "the address links in mail begin with, such as https://cloud.example.com"},
				{Name: "GUSTO_CLOUD_DOMAIN", Text: "the domain organizations' servers are under"},
				{Name: "GUSTO_CLOUD_ORG_ENV", Text: "a file of KEY=VALUE lines every organization's server starts with; its GUSTO_DATABASE_URL, when it is a postgres:// address, gives each organization a schema of its own for its indexes"},
				{Name: "GUSTO_CLOUD_INDEX_DIR", Text: "where organizations' indexes are kept when that address is not Postgres: a directory for each, on this host's local disk (default index under GUSTO_CLOUD_ROOT)"},
			}, envCloudDatabase...),
			Examples: []string{"gusto cloud", "gusto cloud migrate", "gusto cloud workspaces"},
			SeeAlso:  []string{"host", "operator"},
			Run:      none("cloud", cloudCommand),
			Subs: []*cmdline.Command{{
				Name: "workspaces", Summary: "List every workspace on this host",
				Description: "Lists every workspace on this host, with the organization each is in and whose it is: the operator's " +
					"view across organizations, read from each one's own record.",
				Env:      envCloudDatabase,
				Examples: []string{"gusto cloud workspaces"},
				SeeAlso:  []string{"cloud"},
				Run:      none("cloud workspaces", cloudWorkspaces),
			}, cloudMigrateCommand()},
		},
		{
			Name: "host", Group: groupRun, Summary: "Run a host for organizations' servers, for a gusto cloud",
			Description: "Runs the agent on a host that runs organizations' servers for a gusto cloud on another. It starts and stops " +
				"what gusto cloud asks, signed with the secret they share, and says every few seconds that it is there.",
			Env: []cmdline.Param{
				{Name: "GUSTO_CELL_NAME", Text: "what this host is called: lower case letters, digits and hyphens, up to 31"},
				{Name: "GUSTO_CELL_PRIVATE", Text: "this host's IPv4 address on the private network, which its servers bind to"},
				{Name: "GUSTO_CELL_PORT", Text: "the port to listen on (default 8060)"},
				{Name: "GUSTO_CELL_SECRET_FILE", Text: "the file with the secret gusto cloud and the hosts sign with"},
				{Name: "GUSTO_CELLS_DIR", Text: "the directory every host and gusto cloud see"},
			},
			SeeAlso: []string{"cloud"},
			Run:     none("host", hostCommand),
		},
		{
			Name: "operator", Group: groupRun, Summary: "Serve the operator's page for a gusto cloud, on loopback",
			Description: "Serves the page an operator manages a gusto cloud from. It listens on the loopback of the machine gusto cloud " +
				"runs on and refuses any other address, so nothing gusto cloud serves can reach it. Open a tunnel to it.",
			Env: append([]cmdline.Param{
				{Name: "GUSTO_OWNER_ADDR", Text: "where to listen; a loopback address (default 127.0.0.1:8090)"},
			}, envCloudDatabase...),
			SeeAlso: []string{"cloud"},
			Run:     none("operator", operatorCommand),
		},
		{
			Name: "scan", Group: groupRun, Summary: "Ask the runner image which agents it carries",
			Description: "Asks the runner image which agents it carries and keeps the answer beside the image, for the servers that " +
				"start after. A host does it before it takes any work, since the first question waits for the gateway to import the image.",
			Env:     append([]cmdline.Param{}, envRuntime...),
			SeeAlso: []string{"agents", "doctor"},
			Run:     none("scan", scanCommand),
		},
		{
			Name: "param", Group: groupRun, Summary: "Print a parameter from AWS Parameter Store, decrypted",
			Usage: []string{"<name>"},
			Description: "Prints one parameter, decrypted, with the credentials the AWS configuration finds. A host of gusto cloud " +
				"reads its secrets with it while it starts.",
			Args:     []cmdline.Param{{Name: "<name>", Text: "the parameter's name, as /gusto/poc/cell-secret"}},
			Examples: []string{"gusto param /gusto/poc/cell-secret"},
			Run:      paramCommand,
		},
		{
			Name: "doctor", Group: groupRun, Summary: "Check this computer can run a task, before one is sent",
			Description: "Checks everything a task needs on this computer and exits 1 when anything is missing, so a deploy can stop on it.",
			Env:         envRuntime,
			Exit: []cmdline.Param{
				{Name: "0", Text: "everything a task needs is here"},
				{Name: "1", Text: "something is missing"},
				{Name: "2", Text: "usage error"},
			},
			SeeAlso: []string{"scan", "agents"},
			Run:     none("doctor", checkMachine),
		},
	}
}

func localCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "install", Summary: "Start gusto at login and open it",
			Description: "Makes gusto a service of yours that starts when you sign in, and opens it. It keeps the PATH of the shell it " +
				"runs in, and names the model keys that shell has without reading them.",
			Run: homeOnly("local install", installLocal),
		},
		{
			Name: "run", Summary: "Run gusto in this terminal; what the service runs",
			Description: "Runs gusto for one person: in their own folder, on the loopback, signed in by its own links.",
			Run:         homeOnly("local run", runLocal),
		},
		{
			Name: "stop", Summary: "Stop the gusto service",
			Run: none("local stop", func() int { return failIf(stopService()) }),
		},
		{
			Name: "uninstall", Summary: "Stop starting gusto at login",
			Description: "Takes the service away. Your data stays in its folder until you remove the folder.",
			Run:         homeOnly("local uninstall", uninstallLocal),
		},
		{
			Name: "link", Summary: "Print a new sign-in link and open nothing",
			Description: "Prints a sign-in link that works once, within five minutes, for a program that shows gusto itself, such as the desktop app.",
			Run:         homeOnly("local link", printLink),
		},
		{
			Name: "use", Summary: "Add a model key from your shell to gusto",
			Usage: []string{"<NAME> [<model>...]"},
			Description: "Adds the key in one of your environment variables to your own gusto, with the models you name, or the one you pick " +
				"from what the provider lists. The key goes to this computer's keychain and nowhere else.",
			Args: []cmdline.Param{
				{Name: "<NAME>", Text: "ANTHROPIC_API_KEY, DEEPSEEK_API_KEY, OPENAI_API_KEY or OPENROUTER_API_KEY"},
				{Name: "<model>", Text: "a model to offer; left out, you pick from what the provider lists"},
			},
			Examples: []string{"gusto local use ANTHROPIC_API_KEY"},
			Run:      atHome(useKey),
		},
		{
			Name: "sandbox", Summary: "Install or remove the virtual machine sandbox",
			Description: "The virtual machine sandbox is the strongest gusto can give a person's tasks.",
			Subs: []*cmdline.Command{
				{
					Name: "install", Summary: "Download and set up the virtual machine sandbox",
					Usage: []string{"[-yes] [-image <file>]"},
					Description: "Downloads and sets up the virtual machines tasks run in. It says what it will do and asks first, unless " +
						"-yes is given.",
					Flags: flagsOf(declareSandboxInstall),
					Run:   atHome(installSandbox),
				},
				{
					Name: "uninstall", Summary: "Remove the virtual machine sandbox",
					Description: "Removes it. Restart gusto and it chooses another sandbox.",
					Run:         homeOnly("local sandbox uninstall", uninstallSandbox),
				},
			},
		},
	}
}
