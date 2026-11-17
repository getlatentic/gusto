-- team is the team of the organization a task is in, which the column workspace held until teams were named so. What team
-- held before, a label the asker typed for spending, is not kept: the task's own record still has it.
UPDATE tasks SET team = workspace;
DROP INDEX tasks_workspace;
ALTER TABLE tasks DROP COLUMN workspace;
