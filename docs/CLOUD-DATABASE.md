# The accounts database

`gusto cloud` keeps its accounts in a database. Several front doors can share one database, and then share the accounts, the links, the demo counts and the limits. [CLOUD.md](CLOUD.md) describes the service.

## Where it is

| `GUSTO_DATABASE_URL` | The accounts are in |
| --- | --- |
| empty | SQLite, in `cloud.db` under `GUSTO_CLOUD_ROOT`. Nothing to install. |
| `sqlite:///path/to/cloud.db` | SQLite, in that file. |
| `postgres://user:password@host/database?search_path=cloud` | Postgres. `search_path` names the schema. The front door makes the schema when it is missing. |

The root stays. Under `orgs/<slug>/` it holds each organization's own files: `data/` (jobs, keys, `quota.json`, `members.json`, `inbox/`, the computers' records), `entry.key`, `server.log` and `home/`. These are files, written as before, and they are not in the database. The mail `outbox/` and the lead hook's queue (`leadhook/`) are also files of one front door.

The database holds the key that signs cookies, which the root's `secret` file held before: protect it as that file was protected. The address and password of the database go to the front door only. An organization's server gets the `GUSTO_DATABASE_URL` that the operator's `GUSTO_CLOUD_ORG_ENV` file names, and never this one.

SQLite and a shared disk: the file uses a write-ahead log, which does not work between two hosts. On a root that sits on a disk that hosts share (EFS), run one front door and put the file on the local disk with `GUSTO_DATABASE_URL=sqlite:///var/lib/gusto/cloud.db`, or use Postgres.

## Tables

Times are UTC. A SQLite column is an integer of nanoseconds since the Unix epoch, a Postgres column is `timestamptz` (how `internal/dbkit` keeps them).

| Table | Holds | What guards it |
| --- | --- | --- |
| `orgs` | one row for each organization: address (`slug`), name, admin, company domain, and, for a demo, its name, company, ready time, end time, whether it was extended and whether the visitor stopped it | primary key `slug`: two sign-ups cannot take one address |
| `people` | one row for each address: created, verified | primary key `email`: an address has one account and one sign-in |
| `memberships` | one row for each (address, organization): role, joined, who invited them, last sign-in there | primary key `(email, org)`: a person is in an organization once, and in as many organizations as invite them. Foreign key to `orgs`. Index on `(org, created)` for an organization's people |
| `invitations` | an unanswered invitation for each (organization, address), with its expiry and the digest of its link | primary key `(org, email)`: inviting again replaces the earlier one |
| `links` | a single-use link: digest of the secret, address, purpose (`verify`, `signin`, `invite`), expiry, and for an invitation the organization, role and sender | primary key `hash`. The secret itself is never kept |
| `leads` | one row for each visitor who started a demo, kept after the demo is gone, with `who` (the address without a `+tag`) and `domain` | primary key `slug`. Indexes on `requested`, `(who, requested)` and `(domain, requested)` make the daily counts a range read |
| `runtime` | where an organization's server is: state, cell, the two ports, error, restarts | primary key `slug`. Unique `(cell, port)` and `(cell, proxy_port)` where the port is set: two organizations on a cell cannot be given one port, even by two front doors |
| `settings` | `demo_limits` (the owner's limits), `demos_paused` (present means paused), `session_key` (the key that signs cookies) | primary key `key` |
| `rate_events` | one row for each event the abuse limits count, with the time it stops counting | `(key, until)` index. Swept with the links |
| `imports` | one row once the old files were loaded, with the counts | primary key `name` |

The schema is versioned under `internal/cloud/migrations/sqlite` and `migrations/postgres`. The version table is `cloud_schema_migrations` and the scope is `cloud`. A database at a newer version than the release knows is refused. `gusto operator` and `gusto cloud workspaces` open the database without migrating it, and refuse one that is not at the version they read.

## What must be one step

A rule that checks a count and then writes is a transaction. SQLite takes the write lock when a transaction starts. On Postgres each such transaction first takes an advisory lock (`pg_advisory_xact_lock`) named for what it protects, so two front doors queue for it, and a transaction that loses a race is run again (`dbkit.InTx`).

| Operation | One step, because |
| --- | --- |
| Sign-up: person, organization, address, directory | inserts into `orgs` and `people` that name their conflict, then the admin's row in `memberships`. The loser waits for the winner's commit and gets no row, so it reports `ErrSlugTaken` or `ErrEmailTaken`. The directory and `quota.json` are made inside the transaction, after the claims, so only the winner touches them. A directory that is left by a crash holds no address: the next sign-up uses it |
| Organizations a company's domain may have unconfirmed | lock `domain:<domain>` around the count and the insert |
| Using a link once | `DELETE FROM links WHERE hash = ? RETURNING …`. A row has one deleter, so one request gets it. A link used for the wrong purpose, or after its time, is spent by the try, as before |
| Accepting an invitation | the organization is locked, the link is deleted, the room is counted again, the membership is inserted, and the account too when the address has none. A refusal after the link was spent (`ErrFull`, or `ErrEmailTaken` for a person already in the organization) is returned after the transaction commits, so the link stays spent |
| Inviting: room counts people and open invitations | lock `org:<slug>` |
| Role changes and removals: an organization keeps its last admin | lock `org:<slug>` around the admin count and the change, so two admins cannot remove each other |
| Demo limits: day, address, company, at once | lock `demos` around the counts and the insert. The counts are read from `leads`, so they hold after a demo is deleted |
| Demo ready, extend, stop | lock `org:<slug>`; the one extension is checked inside it |
| Abuse limits (sign-ups, mail, sign-ups in all, invitations an hour) | `Store.Limiter` counts in `rate_events` under lock `rate:<key>`: delete the expired rows, count, insert. A database that cannot be asked refuses the event |
| The list of people an organization's server reads (`members.json`) | read and written under lock `roster:<slug>`, so the file always holds the later read |
| First import | lock `import` |

## Loading the old files

Earlier releases kept the accounts as files under the root. The first time a database is opened with no row in `imports`, the front door reads those files and loads them in one transaction, then writes the row. `gusto cloud migrate -import` does the same now. It is safe to run twice (the second run sees the row) and safe on two front doors at once (the transaction holds the import lock). The files are never changed, moved or removed, and organizations' directories are found where they were.

| Old file | Becomes | Left out when |
| --- | --- | --- |
| `orgs/<slug>/org.json` | `orgs` | unreadable, or its slug is not its directory's name |
| `people/<hash>.json` | `people` | unreadable, or its organization is not there |
| `orgs/<slug>/invites/<hash>.json` | `invitations` | unreadable or expired |
| `tokens/<hash>.json` | `links` (the file name is the digest) | unreadable or expired. `.spent` files and files that begin with `.` are skipped |
| `demos/<slug>.json` | `leads` | unreadable |
| `orgs/<slug>/runtime.json` | `runtime` | unreadable, or another row holds its slug or ports |
| `demo-limits.json`, `demos.off`, `secret` | `settings` | the database has the setting |

Each record is inserted with "do nothing on conflict", so a row already in the database stands. The counts of what went in and what was left out are in the `imports` row, in the log, and in the output of `gusto cloud migrate`. Do not delete the `imports` row to import again: links that were used since would come back.

To move a running service: stop the earlier release, run `gusto cloud migrate -import`, start this one.

## Organizations' indexes

Each organization's server has an index of its model calls and tasks. It is derived from the organization's files and is rebuilt when it is lost, so it does not belong on a disk that hosts share.

- If the `GUSTO_DATABASE_URL` in the operator's `GUSTO_CLOUD_ORG_ENV` file is a Postgres address, each organization gets the schema `org_<slug>` in it.
- Otherwise each organization gets `sqlite://<index dir>/<slug>/index.db`. The index directory is `GUSTO_CLOUD_INDEX_DIR`, `index` under the root by default. It must be a path that every host running organization servers can make, on that host's local disk. On a host with a shared root, set it to a local path.

## Checks

- `go test -race ./internal/cloud/...` runs every store test on SQLite, and on Postgres too when `GUSTO_TEST_POSTGRES_URL` is set, in a fresh schema each.
- `shared_test.go` opens two or more `Store`s on one database and races invitations, links, addresses, slugs, room, demo counts, rate limits, role changes and ports. Without the Postgres locks these tests fail.
- `import_test.go` loads `internal/cloud/testdata/oldlayout`, which the earlier release's code wrote with a fixed clock (`testdata/oldlayout-generator.go.txt` is the program), and adds a half-written file, a leftover temporary file, an orphan and records that had run out.
- `GUSTO_CLOUD_SCALE=1 go test -v -run TestTheStoreAtScale ./internal/cloud/` seeds 50,000 people, 5,000 organizations and 20,000 leads and prints how long sign-in lookup, sign-up, accepting an invitation, listing leads and a demo-limit check take.
