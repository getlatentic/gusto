# What this is

A place to run agents that proves what they did. You give it a brief, some input and the command that decides whether it worked. It runs the agent you chose, in a container, on the model you chose, inside a budget. It leaves a record anybody can re-check without trusting the machine that produced it. Coding is the first use, not the point.

## Who it is for

A platform or developer-experience lead at a company that runs two or more agent tools and cannot say what they spent or what they changed. gusto answers three questions: what did all our agents cost last month, what did they touch, and which one is better on our codebase.

Model vendors run their own agents, and none of them compares agents against each other. gusto is neutral between them. It is not for individual developers, who are served by a free command line and a cheap subscription.

## What "governed" means

A buyer works down this list. The state column says how far each row is.

| Row | What it has to mean | State |
| --- | --- | --- |
| identity | SSO, and roles for who may run a job, approve one and change policy | An OIDC provider signs people in. Its groups grant two roles: a runner runs work and sees their own, an admin also sees everybody's and changes what agents are given. The approver role waits for a page that approves. The shared password still signs in as an admin |
| credentials | a key per team, added by the proxy and never inside a sandbox | built, one key |
| spend | budgets per team, project and job, with caps that stop a running job | The proxy counts, caps a job and a team, and reports by team, project and job. Not yet measured against a real provider or reconciled with an invoice |
| data | where a job runs and what may leave it. Outbound denied unless an allowlist names it | The proxy denies any host the allowlist does not name, and the container has no network but the proxy's socket. The seal is not yet measured on the box |
| model policy | which models may see which repositories | one rule: restricted code needs a local model |
| isolation | one sandbox a job, never reused, destroyed after | built |
| change control | an agent opens a pull request and never merges | built |
| supply chain | bundles pinned by digest, skills and connectors from an allowlist | planned (getlatentic/planning#283) |
| audit | an immutable record for each job, exportable | The record is built |
| comparison | the same task on two harnesses, verified side by side | `gusto compare` ([COMPARE.md](COMPARE.md)) |

## What is general

Only one file assumes coding: `internal/runner/workspace.go` clones a repository and takes a `git diff`. `gusto ship` has its own git, and that verb is optional.

General: a job as a directory, a harness named by an adapter, one container a job, the gate, hashed evidence with a manifest, budgets, rules and the proxy. Not general: `repo` and `ref` in the job, which make a repository one kind of input, and `diff.patch`, which is partial because any agent that writes files leaves a delta. The gate covers a test suite, a schema check, a judge model wrapped in a script, and a person clicking approve. Nothing about it is about code.

## Terms

See [TERMS.md](TERMS.md). A harness is the program. An adapter is how to run it. A configured harness is the program plus the skills, connectors, prompt and model policy that shape it. Frozen, a configured harness is an image with a digest. A run of one is an instance. An agent that learns writes to a job's output, never to itself. Otherwise the same digest behaves differently and the evidence proves nothing.

A catalog is the parts somebody picks from. A registry is where an assembled configured harness lives, fetched by digest. An assembler takes the parts, freezes them into an image and runs it. gusto builds the assembler and the proof of what it did. A bundle vendors its skills, prompt, model policy and adapter, and references its harness and connectors by pinned version. The model and credentials are never in it. A pinned version can serve different bytes later, so a run records what actually resolved beside the digest it was asked for, and a stranger compares the two.

## Ways in

| Way in | Who uses it | State |
| --- | --- | --- |
| `gusto` on the command line | whoever wrote the repository | built |
| the pages | anybody on the team | built: a task or a thread on an instance, and `/jobs` as the record of every run |
| `POST /api/jobs` | a script, a CI job, another platform | built |
| an MCP server | any agent that speaks MCP: Claude Code, Codex, Cursor, Zed | not started |

The MCP server lets an agent hand a job to a sandbox and get the evidence back.

## Not yet built

| Deferred | Built when |
| --- | --- |
| naming the configured harness by digest (getlatentic/planning#283) | two people share a name and mean different things |
| the package format | a second kind of harness needs one |
| a registry | something needs fetching that a git repository cannot carry |
| a catalog | somebody has to assemble a harness without reading files |
| merges that deploy themselves (getlatentic/planning#279) | a second person deploys, or merges should deploy unattended |

Other uses need more. An agent that writes no code needs inputs rather than a repository, and an artifact rather than a patch. A swarm needs a parent and child relation between jobs and a way to combine what they produce. Bringing or hosting your own needs the package format. A marketplace needs the package, somewhere to fetch it and provenance. Renting one needs metering and runner classes.

## What blocks a user today

Nothing, for one person. The deployed service uses a real model, runs a real agent in a container, holds a conversation across turns and returns a record that verifies on another machine.

A second person is blocked by spending. One shared sign-in lets everybody who can sign in spend the platform credential. The proxy counts each call and refuses a job or a team past its cap. The count reads a usage shape not yet measured against a real provider, and a team is a label anybody can type. The password goes to nobody until a real run proves the count.
