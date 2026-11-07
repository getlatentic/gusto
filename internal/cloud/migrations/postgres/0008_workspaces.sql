-- A workspace is a team's space in an organization, and workspace_members is who is in it. Every organization has the
-- workspace main, named as the organization, which holds everybody who was in the organization before workspaces.
CREATE TABLE workspaces (
    org     TEXT    NOT NULL REFERENCES orgs (slug),
    id      TEXT    NOT NULL,
    name    TEXT    NOT NULL,
    created TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (org, id)
);

CREATE TABLE workspace_members (
    org       TEXT    NOT NULL,
    workspace TEXT    NOT NULL,
    email     TEXT    NOT NULL,
    created   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (org, workspace, email),
    FOREIGN KEY (org, workspace) REFERENCES workspaces (org, id),
    FOREIGN KEY (email, org) REFERENCES memberships (email, org)
);

CREATE INDEX workspace_members_email ON workspace_members (email, org);

INSERT INTO workspaces (org, id, name, created) SELECT slug, 'main', name, created FROM orgs;

INSERT INTO workspace_members (org, workspace, email, created) SELECT org, 'main', email, created FROM memberships;

-- An invitation's link names the workspace the person joins, and an empty one is the workspace main.
ALTER TABLE links ADD COLUMN workspace TEXT NOT NULL DEFAULT '';
