-- demo_ending marks a demo the sweep has begun to end, so a front door that starts again does not make its server again.
ALTER TABLE orgs ADD COLUMN demo_ending BOOLEAN NOT NULL DEFAULT FALSE;
