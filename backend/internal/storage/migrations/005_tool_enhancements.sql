-- Tool version history (code snapshots)
CREATE TABLE IF NOT EXISTS tool_versions (
    id              TEXT PRIMARY KEY,
    tool_id         TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    code            TEXT NOT NULL,
    change_summary  TEXT NOT NULL DEFAULT '',
    created_at      DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tool_versions ON tool_versions(tool_id, version DESC);

-- Tool tags (free-form labels)
CREATE TABLE IF NOT EXISTS tool_tags (
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    tag         TEXT NOT NULL,
    PRIMARY KEY (tool_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_tool_tags ON tool_tags(tag);

-- Saved test cases (reusable parameter sets)
CREATE TABLE IF NOT EXISTS tool_test_cases (
    id          TEXT PRIMARY KEY,
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT 'Default',
    params_json TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tool_test_cases ON tool_test_cases(tool_id);
