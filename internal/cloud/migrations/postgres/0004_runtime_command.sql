-- command is what an organization's server was started with, so a front door of a newer release restarts one of an older.
ALTER TABLE runtime ADD COLUMN command TEXT NOT NULL DEFAULT '';
