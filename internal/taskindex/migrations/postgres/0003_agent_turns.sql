-- A room agent's turn keeps who took it, who handed it on and who pays, and progress orders a task's states so that an older row
-- cannot be written over a newer one.
ALTER TABLE tasks ADD COLUMN agent     TEXT   COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN handed_by TEXT   COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN answers   TEXT   COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN hops      BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN paid_by   TEXT   COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN progress  BIGINT NOT NULL DEFAULT 0;
UPDATE tasks SET progress = CASE state
    WHEN 'queued' THEN 0
    WHEN 'running' THEN CASE WHEN stopped_by <> '' THEN 2 ELSE 1 END
    ELSE 3 END;
