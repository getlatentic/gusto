-- leader is the lease that makes one front door the one that runs the controller, and the epoch that fences the writes of
-- one that has lost it.
CREATE TABLE leader (
    name   TEXT PRIMARY KEY,
    holder TEXT NOT NULL,
    epoch  INTEGER NOT NULL,
    until  INTEGER NOT NULL
);
