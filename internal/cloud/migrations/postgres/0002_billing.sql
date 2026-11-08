-- One row for each organization that has started paying. An organization with no row is not billed and keeps
-- the quota its email address earned. state is pending (waiting for the payer to approve at PayPal), active,
-- past_due or suspended. seats is the quantity the payer approved, and source_time is the PayPal subscription's
-- update_time of the state last applied, so an older answer never overwrites a newer one.
CREATE TABLE billing (
    org             TEXT    PRIMARY KEY REFERENCES orgs (slug),
    state           TEXT    NOT NULL,
    subscription_id TEXT    NOT NULL,
    approve_url     TEXT    NOT NULL DEFAULT '',
    seats           INTEGER NOT NULL,
    failed_payments INTEGER NOT NULL DEFAULT 0,
    source_time     TIMESTAMPTZ,
    updated         TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX billing_subscription ON billing (subscription_id);

-- The webhook events already applied, so a second delivery of one is a no-op.
CREATE TABLE billing_events (
    id       TEXT    PRIMARY KEY,
    kind     TEXT    NOT NULL,
    received TIMESTAMPTZ NOT NULL
);

-- A change of seats somebody asked for. An admin approves it, and PayPal then asks the payer.
-- status is pending, awaiting_payer, applied or declined.
CREATE TABLE seat_proposals (
    id          TEXT    PRIMARY KEY,
    org         TEXT    NOT NULL REFERENCES orgs (slug),
    seats       INTEGER NOT NULL,
    reason      TEXT    NOT NULL,
    source      TEXT    NOT NULL,
    status      TEXT    NOT NULL,
    approve_url TEXT    NOT NULL DEFAULT '',
    created     TIMESTAMPTZ NOT NULL,
    decided     TIMESTAMPTZ,
    decided_by  TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX seat_proposals_org ON seat_proposals (org, created);

-- The digest of the bearer token an organization's agents use to ask about billing.
CREATE TABLE billing_tokens (
    org     TEXT    PRIMARY KEY REFERENCES orgs (slug),
    digest  TEXT    NOT NULL UNIQUE,
    created TIMESTAMPTZ NOT NULL
);
