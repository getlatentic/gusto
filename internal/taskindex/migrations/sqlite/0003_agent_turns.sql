-- A room agent's turn keeps who took it, who handed it on and who pays, and progress orders a task's states so that an older row
-- cannot be written over a newer one.
ALTER TABLE tasks ADD COLUMN agent     TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN handed_by TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN answers   TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN hops      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN paid_by   TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN progress  INTEGER NOT NULL DEFAULT 0;
UPDATE tasks SET progress = CASE state
    WHEN 'queued' THEN 0
    WHEN 'running' THEN CASE WHEN stopped_by <> '' THEN 2 ELSE 1 END
    ELSE 3 END;
