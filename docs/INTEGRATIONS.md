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
