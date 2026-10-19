# Integrations

An integration is a folder in the open [Agent Plugins](https://agent-plugins.org) format, version 1.0.0: a `plugin.json`, Agent Skills under `skills/`, and MCP servers in `mcp.json`. Version 1.1.0 is a working draft and is not read. The format leaves out permissions, distribution and installation. Those are not built yet (getlatentic/planning#531).

## Reading a plugin

`internal/integration` reads a folder and runs nothing in it. The folder comes from a stranger, so these rules hold:

| Case | Result |
| --- | --- |
| `plugin.json` missing, not JSON, for another version, or with a wrong `name`, `author` or field type | the plugin is rejected |
| more than 5000 files, 100 MiB, or 12 folders deep | the plugin is rejected |
| a field the format does not have, or an `extensions` that is not an object | said, and ignored |
| a skill that breaks the Agent Skills rules, or holds a link out of the plugin | that skill is left out |
| `mcp.json` not valid, or for another version | no servers, and the skills stay |
| one server that breaks the rules | that server is left out, and the others stay |
| a link that leaves the plugin's folder | nothing is read through it |

A server's command is one executable (a bare name or `./path`). Its `cwd` begins `./`, `${PLUGIN_ROOT}` or `${PLUGIN_DATA}`. A remote address is https unless it is this computer, and holds no user name or fragment. Placeholders stay as written. The tests check the reader against the format's JSON Schemas in `schema/agent-plugins/1.0.0`.

## Versions

`integration.Store` keeps each imported plugin by name, one folder for each version.

- An imported integration is off. An administrator turns a version on, for everyone.
- A new version does not replace the one that is on. `Compare` lists the skills and servers it adds, removes or changes, and the descriptive fields that differ. The administrator reads that before turning it on.
- `Restore` turns on the version that was on before. `Disable` turns the integration off and keeps every version.
- A version is named by a fingerprint of its files, so the same files imported twice are one version. A git import is pinned to its commit.
- The kept files hold no link, and version control's own folder is not kept.
- Every change is in the integration's history, with who and when.

## Getting a plugin in

A plugin arrives as an upload or from a git address. Both are a stranger's files.

An upload is a `.zip` or `.tar.gz` of up to 50 MiB. This project's own code unpacks it. It refuses an absolute name, a drive, a name that climbs out, a link, a device, a pipe and a name written twice. It stops at 5000 files or 100 MiB. A refused archive leaves nothing behind. A single top folder is taken off.

A git address must be https with no user name or password. Its host must not resolve to a private, loopback or link-local address unless an administrator allows it. A branch, tag or commit is fetched at depth one, and a folder in the repository may be named. Git runs with none of the machine's configuration, no questions, no hooks, no submodules and no redirects. A link in the repository is written as a plain file.

## Using them

The Integrations page (`/admin/integrations`) is for administrators. It imports from a git address or an upload, lists each integration's versions and sources, shows what a version holds and leaves out, and turns a version on, turns the integration off, or restores the earlier version. Choosing another version shows what would change. Each import and change is in the change record.

`GUSTO_INTEGRATIONS` is the directory, a stamped data directory ([UPGRADE.md](UPGRADE.md)). `GUSTO_INTEGRATIONS_PRIVATE=1` lets a git address be on a private network. The server needs `git`, which its image has.

A setup names the integrations it is given. For each one that is on, a task gets:

- every skill, as a whole folder with its scripts and references, where the agent reads skills
- every streamable HTTP server, behind the gateway ([GATEWAY.md](GATEWAY.md)). The agent is given one server, `gateway`, and finds tools with `tool_search`. A server is called `<integration>-<server>` and a tool `<integration>-<server>/<tool>`

A server on the older SSE transport is held back, and the setup's page says so. A program server (stdio) runs only when an administrator allows the programs of that version, with a ticked box on the integration's page. A new version needs the permission again. The program runs in the gateway's box, never the task's.

A setup that names an integration that is off starts no task. Two skills of one name, or two servers whose names become one, stop the task. An agent that cannot take skills or servers is told before the task starts. `job.json` records each integration's fingerprint, plugin version and pinned commit. It holds no server address or header.

## Who it is on for, and skill sets

An administrator may limit an integration to people by sign-in address: `ada@acme.example`, or `*@acme.example` for a company. None named is everybody. A task for somebody else, or for a sign-in that names nobody, is refused when its setup names the integration.

A skill set made in the console is off until an administrator turns it on, like an integration, a model provider and a model. A skill set kept before there was an off switch is on, and so are the ones gusto ships. A changed or removed skill is kept up to ten times, and the page restores one. The change record shows a short fingerprint of the old and new skill, never the text.

A skill set is a plugin with no servers. "Download as an Agent Plugin" on its page gives a zip that another deployment imports as an integration.

## Skills on each agent

A skill is placed as its whole folder, so `references/`, `scripts/` and `assets/` go with its `SKILL.md`. Measured on 2026-10-08 with a skill whose answer is in `references/word.md`:

| Agent | Reads skills from | Result |
| --- | --- | --- |
| Codex 0.154 | `$CODEX_HOME/skills` | found the skill, read its reference, answered |
| OpenCode 1.18 | `$XDG_CONFIG_HOME/opencode/skills` | found the skill, read its reference, answered |
| Claude Code 2.1.270 | `$CLAUDE_CONFIG_DIR/skills` | in a project's `.claude/skills`, read the reference in 3 of 3 calls. Its own folder was not measured |
| Pi | `~/.pi/agent/skills` | not measured |
| Hermes, DeepSeek agent (`dsh`) | `~/.agents/skills` | not measured |

`GUSTO_MEASURE=1 go test -tags realharnesses -run RealSkill -v ./internal/runner` repeats it for the agents that are signed in.

## In OpenShell sandboxes

`deploy/aws/openshell/e2e-integrations.sh` ran the whole path on 2026-10-08. A plugin with a skill, a reference file and an MCP server was imported, turned on and given to a skill set. Claude Code and Codex each found the tool, called it, read the reference and answered with an id only the customer system holds. The customer system's key appears in nothing the task wrote, and the signed records verify. An agent that reads servers from a file in its own folder (Pi) is refused in a sandbox, by name.
