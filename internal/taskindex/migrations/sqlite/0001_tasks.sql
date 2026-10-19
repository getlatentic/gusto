CREATE TABLE tasks (
    id          TEXT    NOT NULL PRIMARY KEY,
    state       TEXT    NOT NULL,
    brief       TEXT    NOT NULL DEFAULT '',
    repo        TEXT    NOT NULL DEFAULT '',
    harness     TEXT    NOT NULL DEFAULT '',
    model       TEXT    NOT NULL DEFAULT '',
    instance    TEXT    NOT NULL DEFAULT '',
    thread      TEXT    NOT NULL DEFAULT '',
    asked       TEXT    NOT NULL DEFAULT '',
    asked_lc    TEXT    NOT NULL DEFAULT '',
    request_key TEXT    NOT NULL DEFAULT '',
    stopped_by  TEXT    NOT NULL DEFAULT '',
    team        TEXT    NOT NULL DEFAULT '',
    seq         INTEGER NOT NULL,
    exit_code   INTEGER NOT NULL DEFAULT 0,
    queued      INTEGER NOT NULL,
    started     INTEGER NOT NULL DEFAULT 0,
    ended       INTEGER NOT NULL DEFAULT 0,
    search      TEXT    NOT NULL,
    sum         TEXT    NOT NULL
);

CREATE INDEX tasks_queued ON tasks (queued, seq, id);
CREATE INDEX tasks_ended ON tasks (ended, id);
CREATE INDEX tasks_seq ON tasks (seq, id);
CREATE INDEX tasks_asked ON tasks (asked_lc, queued, seq, id);
CREATE INDEX tasks_state ON tasks (state, queued, seq, id);
CREATE INDEX tasks_agent_type ON tasks (harness, queued, seq, id);
CREATE INDEX tasks_repo ON tasks (repo, queued, seq, id);
CREATE INDEX tasks_team ON tasks (team, queued, seq, id);
CREATE INDEX tasks_instance ON tasks (instance, queued, seq, id);
CREATE INDEX tasks_key ON tasks (asked_lc, request_key) WHERE request_key <> '';
