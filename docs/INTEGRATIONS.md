# Integrations

An integration is a folder in the open [Agent Plugins](https://agent-plugins.org) format, version
1.0.0: a `plugin.json`, Agent Skills under `skills/`, and MCP servers in `mcp.json`.
Version 1.1.0 is a working draft and is not read. The format leaves out permissions,
distribution and installation. Those are this project's, and are not built yet
(getlatentic/planning#531).

## Reading a plugin

`internal/integration` reads a folder and runs nothing in it. The folder comes from a stranger,
so these hold:

| Case | Result |
| --- | --- |
| `plugin.json` missing, not JSON, for another version, or with a wrong `name`, `author` or field type | the plugin is rejected |
| more than 5000 files, 100 MiB, or 12 folders deep | the plugin is rejected |
| a field the format does not have, or an `extensions` that is not an object | said, and ignored |
| a skill that breaks the Agent Skills rules, or holds a link out of the plugin | that skill is left out |
| `mcp.json` not valid, or for another version | no servers, and the skills stay |
| one server that breaks the rules | that server is left out, and the others stay |
| a link that leaves the plugin's folder | nothing is read through it |

The server rules are the format's text, which is stricter than its schema: a command is one
executable (a bare name or `./path`), a `cwd` begins `./`, `${PLUGIN_ROOT}` or `${PLUGIN_DATA}`,
a remote address is https unless it is this computer, and no address holds a user name or a
fragment. Placeholders such as `${PLUGIN_ROOT}` are kept as written for whatever runs the server.

The tests check the reader against the format's own JSON Schemas (`schema/agent-plugins/1.0.0`):
nothing the schema refuses is taken, and everything the format shows as valid is.

## Keeping versions

`integration.Store` keeps each imported plugin by name, one folder for each version, with a file
that says which version is on.

- An imported integration is **off**. An administrator turns a version on, for everyone.
- A new version does not replace the one that is on. `Compare` says which skills and servers it
  adds, removes or changes, and which descriptive fields differ, so the administrator reads that
  before turning it on.
- `Restore` turns on the version that was on before. `Disable` turns it off, and keeps every version.
- A version is named by a fingerprint of its files, so the same files imported twice are one
  version. A version is also pinned to where it came from: for a git address, the commit.
- The kept files hold no link, so nothing in them points out of them, and version control's own
  folder is not kept.
- Every change is in the integration's own history, with who and when. A record that cannot be read
  is an error and not an empty list.

## Getting a plugin in

A plugin arrives as an upload or from a git address, and in both it is a stranger's files.

**An upload** is a `.zip` or a `.tar.gz` of up to 50 MiB. It is unpacked by this project's own
code and not a tool's: a name that is absolute, has a drive, or climbs out is refused; a link,
a device and a pipe are refused; a name written twice is refused; and files, folders and bytes
are counted as they are written, so a small archive that unpacks to a great deal stops at 5000
files or 100 MiB. A refused archive leaves nothing behind. An archive made from a repository,
which puts everything in one folder, has that folder taken off.

**A git address** must be https, with no user name or password. The host must not resolve to a
private, loopback or link-local address unless an administrator allows it, so the address cannot
be used to look at what the server can see and the internet cannot. A branch, a tag or a commit
is fetched at depth one, and a folder in the repository may be named. The version is pinned to
the commit that was fetched. Git runs with none of the machine's configuration, no questions, no
hooks, no submodules and no redirects, and a link in the repository is written as a plain file,
so a repository cannot point out of its own folder.

Both fetches only produce a folder. The folder is then read and kept as in the sections above.

## Using them

**The console** has an Integrations page for an administrator (`/admin/integrations`). It imports
from a git address or an upload, lists the versions of each integration with where each came from,
shows what a version holds and what it left out, and turns a version on, turns the integration
off, or puts the earlier version back. Choosing a version other than the one that is on shows
what would change: skills added, removed or changed, servers added, removed or changed, and the
descriptive fields. Each import and each change is in the change record with what was on and what
is on now. `HAAS_INTEGRATIONS` is the directory (a stamped data directory, in
[UPGRADE.md](UPGRADE.md)), and `HAAS_INTEGRATIONS_PRIVATE=1` lets a git address be on a private
network. The server needs `git`, which its image has.

**A setup** names the integrations it is given. A job gets, for each one that is on:

- every skill, as a whole folder with its scripts and references, in the place the harness reads skills;
- every streamable HTTP server, behind the gateway ([GATEWAY.md](GATEWAY.md)): the harness is given
  one server, `gateway`, and finds a tool with `tool_search` when it needs one. The server's address
  and credentials stay with the proxy and are not in the sandbox. A server is called
  `<integration>-<server>`, and a tool `<integration>-<server>/<tool>`.

A server that uses the older SSE transport is held back, and the setup's page says so.

A server that is a program (stdio) runs only when an administrator allows the programs of that
version, on the integration's page, with a box ticked to say they have read what the programs are.
The permission is for that version: a new version is a new program, and is held back until it is
allowed again. The program runs in a box of its own, started by the gateway, and never in the
job's: [GATEWAY.md](GATEWAY.md) says what the box is. A setup that names an integration that is off starts no job, and the
page says why. Two skills of one name, or two servers whose names become one, stop the job and
overwrite nothing. A harness that cannot take skills or servers is told so before the job starts.

`job.json` says, for each integration, the version (its fingerprint and the version the plugin
calls itself) and the commit it was pinned to. The servers' addresses and headers stay out of it.

## Who it is on for, and skill sets

An administrator may choose who an integration is on for, by the address people sign in with: `ada@acme.example`, or
`*@acme.example` for a company. None named is everybody. A job for somebody else, or for a shared sign-in or a program's token that
names nobody, is refused when its setup names the integration, and the page says why. It is by person and not by team because a team
is a label a person chooses for a job, and nothing proves they belong to it.

A skill set an administrator makes in the console is **off** until an administrator turns it on, as an integration, a model provider and
a model are: it is not offered to people, and no agent starts with it. A skill set kept before there was an off switch is on, and so
are the ones haas ships. A skill that is changed or removed is kept as it was, up to ten times, and the page puts one back as a change
of its own. The change record says, for a skill, what it was and what it became as a short fingerprint of each, never the text.

A skill set is a plugin with no servers. **Download as an Agent Plugin** on a skill set's page gives a zip that another deployment, or
this one, imports as an integration like any other. The skill sets a deployment keeps for itself stay as they are: they are edited
in place and are on for everybody, which an integration is not until an administrator turns it on.

## Skills on each harness

A skill is placed as its whole folder, so the `references/`, `scripts/` and `assets/` beside its `SKILL.md` go with it, in the folder
the harness's adapter names. Measured on 2026-10-08 with a skill that says its answer is in `references/word.md`, through the path a
job takes (import, setup, carry, place), and a question that only the skill can answer:

| Harness | Reads skills from | Result |
| --- | --- | --- |
| Codex 0.154 | `$CODEX_HOME/skills` | found the skill, read its reference, answered |
| OpenCode 1.18 | `$XDG_CONFIG_HOME/opencode/skills` | found the skill through its `skill` tool, read its reference, answered |
| Claude Code 2.1.270 | `$CLAUDE_CONFIG_DIR/skills` | in a project's `.claude/skills`, which it reads the same way: listed the skill, and read its reference in 3 of 3 calls. Its own folder was not measured, since Claude Code's login belongs to that folder and the one made for the test is not signed in |
| Pi | `~/.pi/agent/skills` | not measured: not signed in to a model here |
| Hermes | `~/.agents/skills` | not measured: not connected to a provider here |
| DeepSeek harness (`dsh`) | `~/.agents/skills` | not measured: no key here |

`HAAS_MEASURE=1 go test -tags realharnesses -run RealSkill -v ./internal/runner` repeats it for the harnesses that are signed in. It
makes short calls to their models. A small model may not use a skill it has, which is the model's choice and not the placement's.
