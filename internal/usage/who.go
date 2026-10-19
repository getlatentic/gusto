package usage

import "github.com/getlatentic/gusto/internal/jobs"

// WhoOfTask is who made a task's calls, from the task's record: the person who asked, the team and repository,
// the program that ran it, and the conversation, or the task itself when it is in none. engine gives the name a
// person reads for an agent program, and agent the name of the agent a person set up, for an instance.
// The server and the commands that run without one both find who made a call here.
func WhoOfTask(record jobs.Record, engine func(harness string) string, agent func(instance string) (string, bool)) Who {
	who := Who{Session: record.ID, Person: record.Asked, Team: record.Team, Project: record.Repo, Engine: engine(record.Harness)}
	if record.Instance == "" {
		return who
	}
	who.Session = record.Instance
	if record.Thread != "" {
		who.Session += ":" + record.Thread
	}
	if name, found := agent(record.Instance); found {
		who.Agent = name
	}
	return who
}

// AgentName is what an agent is called: the name its person gave it, else its template's title, else its engine's
// name, else "Agent".
func AgentName(title, template, engine string) string {
	for _, name := range []string{title, template, engine} {
		if name != "" {
			return name
		}
	}
	return "Agent"
}
