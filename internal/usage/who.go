package usage

import "github.com/getlatentic/gusto/internal/jobs"

// WhoOfTask is who made a task's calls, from the task's record: the person who asked, the team and repository,
// the agent type that ran it, and the conversation, or the task itself when it is in none. engine gives the name a
// person reads for an agent type, and agent the name of the agent a person set up, for an instance, or of a room's agent that
// took the turn, by its id in the room.
// The server and the commands that run without one both find who made a call here.
func WhoOfTask(record jobs.Record, engine func(harness string) string, agent func(instance, roomAgent string) (string, bool)) Who {
	who := Who{Session: SessionOf(record), Person: record.Payer(), Team: record.Team, Project: record.Repo, Engine: engine(record.Harness)}
	if record.Instance == "" {
		return who
	}
	if name, found := agent(record.Instance, record.Agent); found {
		who.Agent = name
	}
	return who
}

// SessionOf is the conversation a task's calls belong to: the instance and thread when the task is a turn in one, and the task
// itself otherwise.
func SessionOf(record jobs.Record) string {
	switch {
	case record.Instance == "":
		return record.ID
	case record.Thread == "":
		return record.Instance
	}
	return record.Instance + ":" + record.Thread
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
