# The parts

A job is a directory. Every part reads it and writes back to it. It is the only thing the parts share, so a runner is replaceable: hand the same directory to a managed fleet, a container in the customer's account, or a laptop.

| part | its one job | state |
| --- | --- | --- |
| job | a directory holding the brief, the repository, the gate, and what may be spent | built |
| harness | the agent, a program named by a JSON adapter that travels in the job | built |
| sandbox | the box one job runs in, with the runner inside it, destroyed after | built |
| gate | the command that says pass or fail, and the only thing that gives a verdict | built |
| evidence | what the job leaves behind, hashed, checkable by anyone | built |
| rules | who may run what, where, on which model, up to what spend | one rule built |
| ui | the pages a person uses: submit a job, watch it, read the record | Sprint 1 |
| queue | holds jobs, hands one to a runner, restarts a job whose runner dies | Sprint 1 |
| proxy | the only way out: adds the key, counts the tokens, refuses the rest | built; the seal is not yet measured on the box |
| pane | a live terminal for a job, rather than a stream of lines | backlog |

The UI ships inside the same binary, server-rendered, so a customer installs one artifact.

## Ways in

Each way in is a caller of the same job.

| way in | who uses it | when |
| --- | --- | --- |
| `gusto` on the command line | the person who wrote the repository | built |
| the pages | anybody on the team | Sprint 1 |
| the HTTP API | a script, a CI job, another platform | Sprint 1 |
| Slack | somebody who will not open a dashboard | after Sprint 2 |
| the desktop app | somebody who wants to take a job over while it runs | with the pane |

## What the runner never holds

- A model credential. The proxy adds it on the way out.
- A repository credential. The runner clones before the container starts.
- Another tenant's anything. One sandbox a job, destroyed after it.
