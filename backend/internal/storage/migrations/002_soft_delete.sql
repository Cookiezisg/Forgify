-- SQLite cannot ALTER CHECK constraints, so we recreate the tools table.
-- This is safe because we preserve all data via INSERT...SELECT.

CREATE TABLE IF NOT EXISTS tools_new (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    display_name     TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    code             TEXT NOT NULL DEFAULT '',
    requirements     JSON NOT NULL DEFAULT '[]',
    parameters       JSON NOT NULL DEFAULT '[]',
    category         TEXT NOT NULL DEFAULT 'other',
    status           TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','tested','failed','deleted')),
    builtin          BOOLEAN NOT NULL DEFAULT FALSE,
    version          TEXT NOT NULL DEFAULT '1.0',
    requires_key     TEXT,
    pending_code     TEXT,
    pending_summary  TEXT,
    last_test_at     DATETIME,
    last_test_passed BOOLEAN,
    created_at       DATETIME DEFAULT (datetime('now')),
    updated_at       DATETIME DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO tools_new SELECT
    id, name, display_name, description, code, requirements, parameters,
    category, status, builtin, version, requires_key, pending_code, pending_summary,
    last_test_at, last_test_passed, created_at, updated_at
FROM tools;

DROP TABLE tools;
ALTER TABLE tools_new RENAME TO tools;

CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_name ON tools(name);
