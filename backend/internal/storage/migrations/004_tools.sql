-- Tools table
CREATE TABLE IF NOT EXISTS tools (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    code            TEXT NOT NULL DEFAULT '',
    requirements    JSON NOT NULL DEFAULT '[]',
    parameters      JSON NOT NULL DEFAULT '[]',
    category        TEXT NOT NULL DEFAULT 'other',
    status          TEXT NOT NULL DEFAULT 'draft'
                        CHECK(status IN ('draft','tested','failed')),
    builtin         BOOLEAN NOT NULL DEFAULT FALSE,
    version         TEXT NOT NULL DEFAULT '1.0',
    requires_key    TEXT,
    last_test_at    DATETIME,
    last_test_passed BOOLEAN,
    created_at      DATETIME DEFAULT (datetime('now')),
    updated_at      DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_status   ON tools(status);
CREATE INDEX IF NOT EXISTS idx_tools_name     ON tools(name);

-- Tool test history
CREATE TABLE IF NOT EXISTS tool_test_history (
    id              TEXT PRIMARY KEY,
    tool_id         TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    passed          BOOLEAN NOT NULL,
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    input_json      TEXT,
    output_json     TEXT,
    error_msg       TEXT,
    created_at      DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tool_test_history_tool ON tool_test_history(tool_id, created_at DESC);
