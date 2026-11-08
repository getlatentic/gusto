-- quota_due marks an organization whose quota file may not say what the database does. ApplyQuota clears it when it writes the
-- file, and the leader writes every quota still marked.
ALTER TABLE orgs ADD COLUMN quota_due BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX orgs_quota_due ON orgs (slug) WHERE quota_due;
