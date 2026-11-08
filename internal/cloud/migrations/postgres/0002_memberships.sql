CREATE TABLE memberships (
    email        TEXT    NOT NULL,
    org          TEXT    NOT NULL REFERENCES orgs (slug),
    role         TEXT    NOT NULL,
    created      TIMESTAMPTZ NOT NULL,
    invited_by   TEXT    NOT NULL DEFAULT '',
    last_sign_in TIMESTAMPTZ,
    PRIMARY KEY (email, org)
);

CREATE INDEX memberships_org ON memberships (org, created);

INSERT INTO memberships (email, org, role, created, invited_by, last_sign_in)
    SELECT email, org, role, created, invited_by, last_sign_in FROM people;

DROP INDEX people_org;
ALTER TABLE people DROP COLUMN org, DROP COLUMN role, DROP COLUMN invited_by, DROP COLUMN last_sign_in;
