-- workspace is the workspace of the organization a task is in. Every task kept before workspaces is in the first, main.
ALTER TABLE tasks ADD COLUMN workspace TEXT NOT NULL DEFAULT 'main';
CREATE INDEX tasks_workspace ON tasks (workspace, queued, seq, id);
