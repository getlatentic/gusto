CREATE TABLE usage_calls (
    task          TEXT    NOT NULL,
    line          INTEGER NOT NULL,
    called_at     INTEGER NOT NULL,
    session       TEXT    NOT NULL,
    person        TEXT    NOT NULL,
    team          TEXT    NOT NULL DEFAULT '',
    project       TEXT    NOT NULL DEFAULT '',
    engine        TEXT    NOT NULL,
    agent         TEXT    NOT NULL DEFAULT '',
    provider      TEXT    NOT NULL,
    credential    TEXT    NOT NULL DEFAULT '',
    model         TEXT    NOT NULL,
    input_tokens  INTEGER,
    output_tokens INTEGER,
    cached_tokens INTEGER,
    usd           REAL,
    status        INTEGER NOT NULL DEFAULT 0,
    took_ms       INTEGER NOT NULL DEFAULT 0,
    refused       TEXT    NOT NULL DEFAULT '',
    translated    TEXT    NOT NULL DEFAULT '',
    search        TEXT    NOT NULL,
    PRIMARY KEY (task, line)
) WITHOUT ROWID;

CREATE INDEX usage_calls_session ON usage_calls (session, called_at, task, line);
CREATE INDEX usage_calls_at ON usage_calls (called_at, session);
CREATE INDEX usage_calls_person ON usage_calls (person, called_at, session);
CREATE INDEX usage_calls_team ON usage_calls (team, called_at, session);
CREATE INDEX usage_calls_project ON usage_calls (project, called_at, session);
CREATE INDEX usage_calls_engine ON usage_calls (engine, called_at, session);
CREATE INDEX usage_calls_agent ON usage_calls (agent, called_at, session);
CREATE INDEX usage_calls_provider ON usage_calls (provider, called_at, session);
CREATE INDEX usage_calls_model ON usage_calls (model, called_at, session);
