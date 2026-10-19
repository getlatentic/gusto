CREATE TABLE orgs (
    slug          TEXT    PRIMARY KEY,
    name          TEXT    NOT NULL,
    admin         TEXT    NOT NULL,
    domain        TEXT    NOT NULL DEFAULT '',
    created       TIMESTAMPTZ NOT NULL,
    is_demo       BOOLEAN NOT NULL DEFAULT FALSE,
    demo_name     TEXT    NOT NULL DEFAULT '',
    demo_company  TEXT    NOT NULL DEFAULT '',
    demo_ready    TIMESTAMPTZ,
    demo_expires  TIMESTAMPTZ,
    demo_extended BOOLEAN NOT NULL DEFAULT FALSE,
    demo_stopped  BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX orgs_domain ON orgs (domain) WHERE domain <> '';
CREATE INDEX orgs_demo ON orgs (slug) WHERE is_demo;

CREATE TABLE people (
    email        TEXT    PRIMARY KEY,
    org          TEXT    NOT NULL REFERENCES orgs (slug),
    role         TEXT    NOT NULL,
    created      TIMESTAMPTZ NOT NULL,
    verified     TIMESTAMPTZ,
    invited_by   TEXT    NOT NULL DEFAULT '',
    last_sign_in TIMESTAMPTZ
);

CREATE INDEX people_org ON people (org, created);

CREATE TABLE invitations (
    org        TEXT    NOT NULL REFERENCES orgs (slug),
    email      TEXT    NOT NULL,
    role       TEXT    NOT NULL,
    invited_by TEXT    NOT NULL,
    created    TIMESTAMPTZ NOT NULL,
    expires    TIMESTAMPTZ NOT NULL,
    link       TEXT    NOT NULL,
    PRIMARY KEY (org, email)
);

CREATE TABLE links (
    hash       TEXT    PRIMARY KEY,
    email      TEXT    NOT NULL,
    purpose    TEXT    NOT NULL,
    expires    TIMESTAMPTZ NOT NULL,
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
    requested TIMESTAMPTZ NOT NULL,
    ready     TIMESTAMPTZ,
    ended     TIMESTAMPTZ,
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
    port       BIGINT NOT NULL DEFAULT 0,
    proxy_port BIGINT NOT NULL DEFAULT 0,
    error      TEXT    NOT NULL DEFAULT '',
    restarts   BIGINT NOT NULL DEFAULT 0,
    updated    TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX runtime_port ON runtime (cell, port) WHERE port > 0;
CREATE UNIQUE INDEX runtime_proxy_port ON runtime (cell, proxy_port) WHERE proxy_port > 0;

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE rate_events (
    key   TEXT    NOT NULL,
    until TIMESTAMPTZ NOT NULL
);

CREATE INDEX rate_events_key ON rate_events (key, until);

CREATE TABLE imports (
    name   TEXT    PRIMARY KEY,
    ran_at TIMESTAMPTZ NOT NULL,
    counts TEXT    NOT NULL
);

