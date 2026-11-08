CREATE TABLE billing (
    org          TEXT    PRIMARY KEY REFERENCES orgs (slug),
    customer     TEXT    NOT NULL DEFAULT '',
    subscription TEXT    NOT NULL DEFAULT '',
    item         TEXT    NOT NULL DEFAULT '',
    status       TEXT    NOT NULL DEFAULT '',
    seats        INTEGER NOT NULL DEFAULT 0,
    event_at     TIMESTAMPTZ NOT NULL
);

CREATE INDEX billing_billed ON billing (org) WHERE item <> '';
