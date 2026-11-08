CREATE TABLE memberships (
    email        TEXT    NOT NULL,
    org          TEXT    NOT NULL REFERENCES orgs (slug),
    role         TEXT    NOT NULL,
    created      INTEGER NOT NULL,
    invited_by   TEXT    NOT NULL DEFAULT '',
    last_sign_in INTEGER,
    PRIMARY KEY (email, org)
);

CREATE INDEX memberships_org ON memberships (org, created);

INSERT INTO memberships (email, org, role, created, invited_by, last_sign_in)
    SELECT email, org, role, created, invited_by, last_sign_in FROM people;

CREATE TABLE people_only (
    email    TEXT    PRIMARY KEY,
    created  INTEGER NOT NULL,
    verified INTEGER
);

INSERT INTO people_only (email, created, verified) SELECT email, created, verified FROM people;

DROP INDEX people_org;
DROP TABLE people;
ALTER TABLE people_only RENAME TO people;
