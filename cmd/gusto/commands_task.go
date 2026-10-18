package main

import "github.com/getlatentic/gusto/cmd/internal/cmdline"

var (
	envModel = []cmdline.Param{
		{Name: "GUSTO_MODEL_URL", Text: "the model endpoint the agent calls"},
		{Name: "GUSTO_MODEL_TOKEN", Text: "the key for that endpoint"},
	}
	envSigning = cmdline.Param{Name: "GUSTO_SIGNING_KEY", Text: "the private key the record is signed with, from gusto keygen"}
	envKeyring = cmdline.Param{Name: "GUSTO_KEYRING", Text: "the directory of public keys that name a record's signer"}
	envRuntime = []cmdline.Param{
		{Name: "GUSTO_CONTAINER", Text: "the container runtime to use, docker or podman"},
		{Name: "GUSTO_IMAGE", Text: "the runner image tasks run in"},
	}
	exitCheck = []cmdline.Param{
		{Name: "0", Text: "the check passed"},
		{Name: "1", Text: "the check failed, or the task could not be run"},
		{Name: "2", Text: "usage error"},
	}
)

func taskCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "new", Group: groupTask, Summary: "Make a task directory: repository, brief and check",
			Usage: []string{"<dir> -repo <url> -brief <text> [-agent <name>] -- <check command>"},
			Description: "Makes the directory a task lives in and writes what it asks for: the repository to work in, what to do, " +
				"the agent and the check. The check is the command whose exit code says pass or fail. Everything after -- is the check.",
			Args:  []cmdline.Param{{Name: "<dir>", Text: "where to make the task"}},
			Flags: flagsOf(declareNew),
			Examples: []string{
				"gusto new fix-login -repo https://github.com/acme/app \\",
				"    -brief \"Fix the redirect after login\" -- npm test",
			},
			SeeAlso: []string{"run", "inside", "agents"},
			Run:     newTask,
		},
		{
			Name: "run", Group: groupTask, Summary: "Do a task in a container",
			Usage: []string{"<dir>"},
			Description: "Checks the repository out here, so that a repository credential stays on this computer and never enters the " +
				"container with the agent, then runs the agent and the check in a container. The exit code is the check's answer.",
			Args:     []cmdline.Param{{Name: "<dir>", Text: "the task directory made by gusto new"}},
			Env:      append(append(append([]cmdline.Param{}, envRuntime...), envModel...), envSigning),
			Examples: []string{"gusto run fix-login"},
			Exit:     exitCheck,
			SeeAlso:  []string{"inside", "verify", "show"},
			Run:      runTask,
		},
		{
			Name: "inside", Group: groupTask, Summary: "Do a task here, without a container",
			Usage:       []string{"<dir>"},
			Description: "Runs the agent and the check in this process, on this computer. Stopping it with an interrupt stops the agent, and the record says so. The exit code is the check's answer.",
			Args:        []cmdline.Param{{Name: "<dir>", Text: "the task directory made by gusto new"}},
			Env:         append(append([]cmdline.Param{}, envModel...), envSigning),
			Examples:    []string{"gusto inside fix-login"},
			Exit:        exitCheck,
			SeeAlso:     []string{"run", "verify"},
			Run:         inside,
		},
		{
			Name: "submit", Group: groupTask, Summary: "Send a task to a server and watch it there",
			Usage:       []string{"<dir>"},
			Description: "Sends the task to a gusto server instead of running it here, so the work outlives your shell. Prints the task's id and its address on the server.",
			Args:        []cmdline.Param{{Name: "<dir>", Text: "the task directory made by gusto new"}},
			Env: []cmdline.Param{
				{Name: "GUSTO_SERVER", Text: "the server, with its scheme, such as https://gusto.example.com"},
				{Name: "GUSTO_TOKEN", Text: "an API token from the server's Tokens page"},
				{Name: "GUSTO_PASSWORD", Text: "the server's shared sign-in, when it has no tokens"},
			},
			Examples: []string{"GUSTO_SERVER=https://gusto.example.com gusto submit fix-login"},
			SeeAlso:  []string{"serve", "show"},
			Run:      submit,
		},
		{
			Name: "show", Group: groupTask, Summary: "What a task asked for and what happened",
			Usage:       []string{"<dir>"},
			Description: "Prints the brief, the repository, the agent and the check, then the verdict, the model, the signer, the files and the cost, once the task has run.",
			Args:        []cmdline.Param{{Name: "<dir>", Text: "the task directory"}},
			Env:         []cmdline.Param{envKeyring},
			Examples:    []string{"gusto show fix-login"},
			SeeAlso:     []string{"verify", "rerun"},
			Run:         show,
		},
		{
			Name: "rerun", Group: groupTask, Summary: "Run finished tasks again and compare the verdicts",
			Usage: []string{"<dir>... [-into <dir>] [-here]"},
			Description: "Runs each finished task again at the commit it started from, and prints one summary of how the verdicts changed. " +
				"Give a task directory, or a directory of tasks.",
			Args:     []cmdline.Param{{Name: "<dir>...", Text: "task directories, or a directory of them"}},
			Flags:    flagsOf(declareRerun),
			Env:      envRuntime,
			Examples: []string{"gusto rerun tasks/", "gusto rerun fix-login -here"},
			Exit: []cmdline.Param{
				{Name: "0", Text: "every verdict held or improved"},
				{Name: "1", Text: "a verdict got worse"},
				{Name: "2", Text: "a task could not be rerun, or a usage error"},
			},
			SeeAlso: []string{"compare", "benchmark"},
			Run:     rerunTasks,
		},
		{
			Name: "compare", Group: groupTask, Summary: "Run one task on two agents, side by side",
			Usage: []string{"<dir> <agent> <agent> [-into <dir>] [-here]", "<comparison>"},
			Description: "Runs one task on each agent as one comparison, or prints a comparison that already ran. " +
				"It exits 0 when every side ran and its record checks, whichever agent did better.",
			Args: []cmdline.Param{
				{Name: "<dir>", Text: "the task directory"},
				{Name: "<agent>", Text: "an agent from gusto agents"},
				{Name: "<comparison>", Text: "a comparison directory that already ran"},
			},
			Flags:    flagsOf(declareCompare),
			Env:      envRuntime,
			Examples: []string{"gusto compare fix-login claude-code codex"},
			Exit: []cmdline.Param{
				{Name: "0", Text: "every side ran and its record checks"},
				{Name: "2", Text: "a side did not, or a usage error"},
			},
			SeeAlso: []string{"agents", "benchmark"},
			Run:     compareTasks,
		},
		{
			Name: "benchmark", Group: groupTask, Summary: "Run tasks on agent and model pairs, as a table",
			Usage: []string{"<plan> [-into <dir>] [-here] [-again <pairs>] [-json]"},
			Description: "Runs a plan's tasks on every agent and model pair it names, and prints a table of which finish, or prints a " +
				"benchmark that already ran. In a container each run goes out as a server's task does, through the proxy when a credential " +
				"is held, so its cost is the proxy's count. It exits 0 when every run left a verdict and its record checks.",
			Args:  []cmdline.Param{{Name: "<plan>", Text: "a plan file, or a benchmark directory that ran"}},
			Flags: flagsOf(declareBenchmark),
			Env: append([]cmdline.Param{{Name: "GUSTO_SANDBOX", Text: "openshell runs each task in an OpenShell sandbox"}},
				envRuntime...),
			Examples: []string{"gusto benchmark plan.json", "gusto benchmark plan.json -again all"},
			Exit: []cmdline.Param{
				{Name: "0", Text: "every run left a verdict and its record checks"},
				{Name: "2", Text: "a run did not, or a usage error"},
			},
			SeeAlso: []string{"compare", "rerun"},
			Run:     benchmarkTasks,
		},
	}
}
