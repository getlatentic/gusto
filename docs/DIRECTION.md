# What this is

A place to run agents that proves what they did.

You give it a brief, some input, and the command that decides whether it worked.
It runs the agent you chose, in a container, on the model you chose, inside a
budget. It leaves a record anybody can re-check without trusting the machine
that produced it.

Coding is the first use, not the point.

## Why there is room

The model vendors are buying the execution layer under agents. OpenAI acquired
Ona in June 2026 to run Codex agents unattended. GitHub sells Codespaces at
$0.18 a core-hour and puts Copilot agents on top. Anthropic runs Claude Code
cloud sessions. Each one runs its own agent.

So none of them will answer the question a platform team actually has: what did
all of our agents cost last month, what did they touch, and which one is better
on our codebase. Neutrality is the one position a vendor cannot take, and it
gets more valuable as they integrate.

## Who buys it

A platform or developer-experience lead at a company already running two or more
agent tools, who cannot say what they spent or what they changed. The opening
question is exactly that, and an answer of "I don't know" is the sale.

Not individual developers. A free command line and a cheap subscription already
serve them, and the vendors bundle hosting with their own agents.

## What "governed" has to mean

This list is the product. A buyer works down it, and most of it is not built.

| | | state |
| --- | --- | --- |
| identity | SSO, and roles that separate who may run a job, who may approve one, and who may change policy | one shared password |
| credentials | a key per team, added by the proxy on the way out and never inside a sandbox | built, one key |
| spend | budgets per team, project and job, with caps that stop a running job | the proxy counts, caps a job and a team, and reports by team, project and job; not yet measured against a real provider or reconciled with an invoice |
| data | where a job runs, and what may leave it. Outbound denied unless an allowlist names it | the proxy denies any host the allowlist does not name, and the container has no network but the proxy's socket; the seal is not yet measured on the box |
| model policy | which models may see which repositories, enforced rather than remembered | one rule: restricted code needs a local model |
| isolation | one sandbox a job, never reused, destroyed after | built |
| change control | an agent opens a pull request and never merges | built |
| supply chain | bundles pinned by digest, skills and connectors from an allowlist | #64 |
| audit | an immutable record a job at a time, exportable | the record is built; the export is not |
| comparison | the same task on two harnesses, verified side by side | Sprint 4 |

The last row is the one nobody else can offer, because everybody else sells one
agent.

## What is general, and what is not

Measured rather than assumed. The coding assumption lives in one file,
`internal/runner/workspace.go`, which clones a repository and takes a `git diff`.
`haas ship` has its own git, and that verb is optional.

| piece | general |
| --- | --- |
| a job is a directory | yes |
| a harness is a program named by an adapter | yes |
| one container a job, destroyed after | yes |
| the gate: a command that exits zero | yes |
| evidence: files, hashed, with a manifest | yes |
| budgets, rules, the proxy | yes |
| `repo` and `ref` in the job | no — these are inputs, and a repository is one kind |
| `diff.patch` | partly — any agent that writes files leaves a delta |

The gate is the surprise. "A command that exits zero" already covers a test
suite, a schema check, a judge model wrapped in a script, and a person clicking
approve. Nothing about it is about code.

## The uses, and what each one adds

| use | what it needs that does not exist |
| --- | --- |
| a coding agent | nothing |
| an agent that writes no code | inputs rather than a repository; the result is an artifact rather than a patch |
| a swarm | a parent and child relation between jobs, and a way to combine what they produce |
| bring or host your own | the package format below |
| a marketplace | the package, plus somewhere to fetch it and provenance for it |
| renting one | metering, and runner classes |

## Three words that keep getting confused

See [TERMS.md](TERMS.md). A **harness** is the program. An **adapter** is how to
run it. A **configured harness** is the program plus the skills, connectors,
prompt and model policy that shape it. People share and sell the third.

Frozen, a configured harness is an **image** with a digest. A run of one is an
**instance**. The rule that keeps this honest: an agent that learns writes to a
job's output, never to itself. Otherwise the same digest behaves differently and
the evidence proves nothing.

## Catalog, registry, assembler

Three different things, and only one of them is ours to build.

- A **catalog** is the parts somebody picks from: harnesses, skills, connectors.
- A **registry** is where an assembled configured harness lives, fetched by digest.
- An **assembler** takes the parts, freezes them into an image, and runs it.

The parts already have homes — skills have a format, connectors have registries,
harnesses ship on npm and Homebrew. Indexing those is a small job. Nobody owns
the assembler, and nobody proves what the assembly then did. That is the part
worth building.

### What a bundle can and cannot contain

An image contains everything it needs. A harness bundle cannot: skills are files
and vendor cleanly, but a connector is a process with its own dependencies and is
sometimes a remote address. So a bundle vendors its skills, prompt, model policy
and adapter, and references its harness and connectors by pinned version. The
model and the credentials are never in it.

That leaves one problem, and it is the reason this is not simply OCI: a digest
over declarations is not proof of what ran. A pinned version can serve different
bytes later and the digest still matches. The answer is the one the rest of this
gives — the bundle declares, the evidence proves. A run records what actually
resolved beside the digest it was asked for, and a stranger compares the two.

## Ways in

The pages are not the main entrance.

| way in | who uses it | state |
| --- | --- | --- |
| `haas` on the command line | whoever wrote the repository | built |
| the pages | anybody on the team | built, including talking to a harness |
| `POST /api/jobs` | a script, a CI job, another platform | built |
| an MCP server | any agent that speaks MCP: Claude Code, Codex, Cursor, Zed | not started |

The MCP server matters more than another page. Somebody already working in Codex
does not want a second web application; they want their agent to hand a job to a
sandbox and get the evidence back. One thing to build, every editor at once, and
it stops this competing with the agent they already like — it sits underneath it.

## What is deferred, and what promotes it

Nothing here is a plan to build soon. Each line says what would make it urgent.

| deferred | promoted when |
| --- | --- |
| naming the configured harness by digest (#64) | two people share a name and mean different things — the first day a team uses this |
| the package format | a second kind of harness genuinely needs one |
| a registry | something needs fetching that a git repository cannot carry |
| a catalog | somebody has to assemble a harness without reading files |
| merges deploying themselves (#52) | a second person deploys, or merges should deploy unattended |

Designing a registry before anything needs registering is how this stops being a
product.

## What actually blocks a user today

Nothing, for one person. The deployed service thinks with a real model, runs a
real agent in a container, holds a conversation across turns, and hands back a
record that verifies on somebody else's machine.

What blocks a *second* person is spending. One shared sign-in means everybody who
can sign in can spend the platform credential. The proxy now counts what each
call cost and refuses a job or a team past its cap, but the count reads a usage
shape nobody has yet measured against a real provider, and a team is a label
anybody can type. The password does not go to anybody until a real run proves
the count.
