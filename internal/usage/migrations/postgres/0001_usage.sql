CREATE TABLE usage_calls (
    task          TEXT             COLLATE "C" NOT NULL,
    line          INTEGER          NOT NULL,
    called_at     TIMESTAMPTZ      NOT NULL,
    session       TEXT             COLLATE "C" NOT NULL,
    person        TEXT             COLLATE "C" NOT NULL,
    team          TEXT             COLLATE "C" NOT NULL DEFAULT '',
    project       TEXT             COLLATE "C" NOT NULL DEFAULT '',
    engine        TEXT             COLLATE "C" NOT NULL,
    agent         TEXT             COLLATE "C" NOT NULL DEFAULT '',
    provider      TEXT             COLLATE "C" NOT NULL,
    credential    TEXT             COLLATE "C" NOT NULL DEFAULT '',
    model         TEXT             COLLATE "C" NOT NULL,
    input_tokens  BIGINT,
    output_tokens BIGINT,
    cached_tokens BIGINT,
    usd           DOUBLE PRECISION,
    status        INTEGER          NOT NULL DEFAULT 0,
    took_ms       BIGINT           NOT NULL DEFAULT 0,
    refused       TEXT             COLLATE "C" NOT NULL DEFAULT '',
    translated    TEXT             COLLATE "C" NOT NULL DEFAULT '',
    search        TEXT             COLLATE "C" NOT NULL,
    PRIMARY KEY (task, line)
);

CREATE INDEX usage_calls_session ON usage_calls (session, called_at, task, line);
CREATE INDEX usage_calls_at ON usage_calls (called_at, session);
CREATE INDEX usage_calls_person ON usage_calls (person, called_at, session);
CREATE INDEX usage_calls_team ON usage_calls (team, called_at, session);
CREATE INDEX usage_calls_project ON usage_calls (project, called_at, session);
CREATE INDEX usage_calls_engine ON usage_calls (engine, called_at, session);
CREATE INDEX usage_calls_agent ON usage_calls (agent, called_at, session);
CREATE INDEX usage_calls_provider ON usage_calls (provider, called_at, session);
CREATE INDEX usage_calls_model ON usage_calls (model, called_at, session);
