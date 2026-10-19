CREATE TABLE tasks (
    id          TEXT   COLLATE "C" NOT NULL PRIMARY KEY,
    state       TEXT   COLLATE "C" NOT NULL,
    brief       TEXT   COLLATE "C" NOT NULL DEFAULT '',
    repo        TEXT   COLLATE "C" NOT NULL DEFAULT '',
    harness     TEXT   COLLATE "C" NOT NULL DEFAULT '',
    model       TEXT   COLLATE "C" NOT NULL DEFAULT '',
    instance    TEXT   COLLATE "C" NOT NULL DEFAULT '',
    thread      TEXT   COLLATE "C" NOT NULL DEFAULT '',
    asked       TEXT   COLLATE "C" NOT NULL DEFAULT '',
    asked_lc    TEXT   COLLATE "C" NOT NULL DEFAULT '',
    request_key TEXT   COLLATE "C" NOT NULL DEFAULT '',
    stopped_by  TEXT   COLLATE "C" NOT NULL DEFAULT '',
    team        TEXT   COLLATE "C" NOT NULL DEFAULT '',
    seq         BIGINT NOT NULL,
    exit_code   BIGINT NOT NULL DEFAULT 0,
    queued      BIGINT NOT NULL,
    started     BIGINT NOT NULL DEFAULT 0,
    ended       BIGINT NOT NULL DEFAULT 0,
    search      TEXT   COLLATE "C" NOT NULL,
    sum         TEXT   COLLATE "C" NOT NULL
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
