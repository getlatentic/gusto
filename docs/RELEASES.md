# What each release changes for an operator

A release's notes list the images and the merged pull requests. This file says what
an operator has to know before they upgrade: settings added, defaults changed, and
things the server now refuses.

A tag needs a section here named for it. The release workflow stops without one and
copies the section into the release's notes. Changes made after a release go under
**Next release**, and that heading is renamed to the tag when the tag is made.
[UPGRADE.md](UPGRADE.md) says what happens to the data, and
[PILOT.md](PILOT.md) says how to install and check a release.

## Next release

**Settings added**

- `HAAS_OPENSHELL_REMOTE=1`: the OpenShell gateway is on another machine, so this one needs no `/dev/kvm`.
- The chart's `auth.oidc` and `spend` values, and the OIDC settings in `deploy/compose/env.example`. The server already took these settings; [SSO.md](SSO.md) now says what they do.

**Refused now**

- `HAAS_SANDBOX=openshell` on a Linux machine with no `/dev/kvm`. The server does not start, and `haas doctor` reports it.

## v0.0.1-alpha.2

Since `v0.0.1-alpha.1`. Nothing to set and nothing new refused.

**Fixed**

- A server that held credentials refused every job that did not name one, including a job whose harness needs no model. `deploy/smoke.sh` failed on such an install.

## v0.0.1-alpha.1

Since `v0.1.0-rc.1`. The data stays at format 1 in every store, and
`haas data rollback -to v0.1.0-rc.1` needs nothing undone.

**Settings added**

- `HAAS_LOCAL_REPOS`: the directories a job's repository may be cloned from. Unset, a repository is an https or ssh address and nothing on the server's own disk. **A deployment that submitted local paths has to set it.**
- `HAAS_RUNNER_GROUPS`, `HAAS_ADMIN_GROUPS`, `HAAS_ADMINS`, `HAAS_OIDC_SCOPES`, `HAAS_OIDC_GROUPS_CLAIM`: roles from the identity provider's groups.
- `HAAS_API_TOKENS` and `HAAS_TOKEN`: scoped, revocable API tokens. With the file set, the API refuses the shared password ([TOKENS.md](TOKENS.md)).
- `HAAS_LIMIT_PERSON`, `HAAS_LIMIT_TEAM`: the most one person or team runs at once ([LIMITS.md](LIMITS.md)).
- `HAAS_RETENTION`: removes finished work on a schedule. With no policy nothing is removed ([RETENTION.md](RETENTION.md)).
- `HAAS_CHANGES`: where the record of what changed on the platform is kept ([CHANGES.md](CHANGES.md)).
- `HAAS_MIGRATE_IRREVERSIBLE`: lets one start run a migration that cannot be undone.
- `HAAS_DEPLOY_RESULT`: a file a deploy writes, reported at `/metrics`.
- `HAAS_SANDBOX=openshell` and `HAAS_OPENSHELL*`: run jobs in OpenShell sandboxes ([OPENSHELL.md](OPENSHELL.md)).
- `HAAS_GITHUB_APP_ID`, `HAAS_GITHUB_APP_KEY`, `HAAS_GITHUB_URL`: a job reaches a private repository with a token for it alone.
- `HAAS_REGISTRY`, `HAAS_ORG_SETTINGS`: an organisation's model registry and settings, set by a front door.
- A credential's `wires` and `maxOutput`: a call in a shape the provider does not answer is translated to Chat Completions ([GATEWAY.md](GATEWAY.md)).
- `HAAS_SIGNING_KEY` takes `awskms://<key>`.

**Defaults changed**

- A job's repository is an https or ssh address, or a directory under `HAAS_LOCAL_REPOS`. It is no longer any path on the server.
- `/readyz` answers 503 when fewer workers run than `HAAS_JOB_CONCURRENCY`, or the jobs directory cannot be written ([RUNBOOK.md](RUNBOOK.md)).

**Refused now**

- A server with an identity provider and neither a group mapping nor `HAAS_ALLOWED` does not start.
- A person the provider signs in whom no group maps to is sent back to the login page.
- Only an admin changes setups and skills.
- A migration that cannot be undone, unless `HAAS_MIGRATE_IRREVERSIBLE=1`.
