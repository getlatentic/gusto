-- A contact is a note a visitor sent the team from the landing page. who and domain are what the limits on how many one
-- address and one company may send in a day count by.
CREATE TABLE contacts (
    id      TEXT    PRIMARY KEY,
    sent    INTEGER NOT NULL,
    email   TEXT    NOT NULL,
    company TEXT    NOT NULL,
    note    TEXT    NOT NULL,
    who     TEXT    NOT NULL,
    domain  TEXT    NOT NULL
);

CREATE INDEX contacts_sent ON contacts (sent);
CREATE INDEX contacts_who ON contacts (who, sent);
CREATE INDEX contacts_domain ON contacts (domain, sent);
