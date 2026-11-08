-- One row for each organization that has started paying, whichever processor it pays through. An organization with
-- no row is not billed and keeps the quota its email address earned. state is pending (nobody has paid yet), active,
-- past_due or suspended. ref is the processor's name for the subscription. seats is the quantity billed. fixed_seats
-- says the seats are what the payer approved and cap the people, where otherwise they follow the people. source_time
-- is when the processor made the answer last applied, so an older answer never overwrites a newer one. paid_once says
-- the subscription was ever paying, so one that ended before anyone paid for it changes no quota.
CREATE TABLE billing_accounts (
    org             TEXT    PRIMARY KEY REFERENCES orgs (slug),
    provider        TEXT    NOT NULL,
    ref             TEXT    NOT NULL,
    customer        TEXT    NOT NULL DEFAULT '',
    handle          TEXT    NOT NULL DEFAULT '',
    state           TEXT    NOT NULL,
    seats           INTEGER NOT NULL,
    fixed_seats     BOOLEAN NOT NULL DEFAULT FALSE,
    approve_url     TEXT    NOT NULL DEFAULT '',
    failed_payments INTEGER NOT NULL DEFAULT 0,
    paid_once       BOOLEAN NOT NULL DEFAULT FALSE,
    source_time     INTEGER,
    updated         INTEGER NOT NULL
);

CREATE UNIQUE INDEX billing_accounts_ref ON billing_accounts (provider, ref);

-- The subscriptions the earlier release kept for Stripe, in the same form.
INSERT INTO billing_accounts (org, provider, ref, customer, handle, state, seats, fixed_seats, source_time, updated, paid_once)
SELECT org, 'stripe', subscription, customer, item,
       CASE WHEN status IN ('active', 'trialing') THEN 'active'
            WHEN status = 'past_due' THEN 'past_due'
            WHEN status IN ('canceled', 'unpaid', 'incomplete_expired', 'paused') THEN 'suspended'
            ELSE 'pending' END,
       seats, FALSE, event_at, event_at,
       status IN ('active', 'trialing', 'past_due', 'canceled', 'unpaid', 'paused')
FROM billing WHERE subscription <> '';

DROP TABLE billing;

-- The notices already applied, so a second delivery of one is a no-op.
CREATE TABLE billing_events (
    provider TEXT    NOT NULL,
    id       TEXT    NOT NULL,
    kind     TEXT    NOT NULL,
    received INTEGER NOT NULL,
    PRIMARY KEY (provider, id)
);

-- A change of seats somebody asked for. An admin approves it, and the processor then asks the payer.
-- status is pending, awaiting_payer, applied or declined.
CREATE TABLE seat_proposals (
    id          TEXT    PRIMARY KEY,
    org         TEXT    NOT NULL REFERENCES orgs (slug),
    seats       INTEGER NOT NULL,
    reason      TEXT    NOT NULL,
    source      TEXT    NOT NULL,
    status      TEXT    NOT NULL,
    approve_url TEXT    NOT NULL DEFAULT '',
    created     INTEGER NOT NULL,
    decided     INTEGER,
    decided_by  TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX seat_proposals_org ON seat_proposals (org, created);

-- The digest of the bearer token an organization's agents use to ask about billing.
CREATE TABLE billing_agent_tokens (
    org     TEXT    PRIMARY KEY REFERENCES orgs (slug),
    digest  TEXT    NOT NULL UNIQUE,
    created INTEGER NOT NULL
);
