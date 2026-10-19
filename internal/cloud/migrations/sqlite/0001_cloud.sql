CREATE TABLE orgs (
    slug          TEXT    PRIMARY KEY,
    name          TEXT    NOT NULL,
    admin         TEXT    NOT NULL,
    domain        TEXT    NOT NULL DEFAULT '',
    created       INTEGER NOT NULL,
    is_demo       BOOLEAN NOT NULL DEFAULT FALSE,
    demo_name     TEXT    NOT NULL DEFAULT '',
    demo_company  TEXT    NOT NULL DEFAULT '',
    demo_ready    INTEGER,
    demo_expires  INTEGER,
    demo_extended BOOLEAN NOT NULL DEFAULT FALSE,
    demo_stopped  BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX orgs_domain ON orgs (domain) WHERE domain <> '';
CREATE INDEX orgs_demo ON orgs (slug) WHERE is_demo;

CREATE TABLE people (
    email        TEXT    PRIMARY KEY,
    org          TEXT    NOT NULL REFERENCES orgs (slug),
    role         TEXT    NOT NULL,
    created      INTEGER NOT NULL,
    verified     INTEGER,
    invited_by   TEXT    NOT NULL DEFAULT '',
    last_sign_in INTEGER
);

CREATE INDEX people_org ON people (org, created);

CREATE TABLE invitations (
    org        TEXT    NOT NULL REFERENCES orgs (slug),
    email      TEXT    NOT NULL,
    role       TEXT    NOT NULL,
    invited_by TEXT    NOT NULL,
    created    INTEGER NOT NULL,
    expires    INTEGER NOT NULL,
    link       TEXT    NOT NULL,
    PRIMARY KEY (org, email)
);

CREATE TABLE links (
    hash       TEXT    PRIMARY KEY,
    email      TEXT    NOT NULL,
    purpose    TEXT    NOT NULL,
    expires    INTEGER NOT NULL,
    org        TEXT    NOT NULL DEFAULT '',
    role       TEXT    NOT NULL DEFAULT '',
    invited_by TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX links_expires ON links (expires);

CREATE TABLE leads (
    slug      TEXT    PRIMARY KEY,
    name      TEXT    NOT NULL,
    email     TEXT    NOT NULL,
    company   TEXT    NOT NULL,
    requested INTEGER NOT NULL,
    ready     INTEGER,
    ended     INTEGER,
    extended  BOOLEAN NOT NULL DEFAULT FALSE,
    reason    TEXT    NOT NULL DEFAULT '',
    who       TEXT    NOT NULL,
    domain    TEXT    NOT NULL
);

CREATE INDEX leads_requested ON leads (requested);
CREATE INDEX leads_who ON leads (who, requested);
CREATE INDEX leads_domain ON leads (domain, requested);

CREATE TABLE runtime (
    slug       TEXT    PRIMARY KEY REFERENCES orgs (slug),
    state      TEXT    NOT NULL,
    cell       TEXT    NOT NULL DEFAULT '',
    port       INTEGER NOT NULL DEFAULT 0,
    proxy_port INTEGER NOT NULL DEFAULT 0,
    error      TEXT    NOT NULL DEFAULT '',
    restarts   INTEGER NOT NULL DEFAULT 0,
    updated    INTEGER NOT NULL
);

CREATE UNIQUE INDEX runtime_port ON runtime (cell, port) WHERE port > 0;
CREATE UNIQUE INDEX runtime_proxy_port ON runtime (cell, proxy_port) WHERE proxy_port > 0;

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE rate_events (
    key   TEXT    NOT NULL,
    until INTEGER NOT NULL
);

CREATE INDEX rate_events_key ON rate_events (key, until);

CREATE TABLE imports (
    name   TEXT    PRIMARY KEY,
    ran_at INTEGER NOT NULL,
    counts TEXT    NOT NULL
);
