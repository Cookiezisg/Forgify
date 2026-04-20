-- Forgify V1.1 Schema (clean slate)

-- App config (key-value store)
CREATE TABLE IF NOT EXISTS app_config (
    key        TEXT PRIMARY KEY,
    value      TEXT,
    updated_at DATETIME DEFAULT (datetime('now'))
);

-- API Keys (encrypted)
CREATE TABLE IF NOT EXISTS api_keys (
    id           TEXT PRIMARY KEY,
    provider     TEXT NOT NULL,
    display_name TEXT,
    key_enc      TEXT NOT NULL,
    base_url     TEXT,
    test_status  TEXT,
    last_tested  DATETIME,
    created_at   DATETIME DEFAULT (datetime('now')),
    updated_at   DATETIME DEFAULT (datetime('now'))
);

-- Conversations
CREATE TABLE IF NOT EXISTS conversations (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL DEFAULT '新对话',
    asset_id   TEXT,
    asset_type TEXT CHECK(asset_type IN ('tool','workflow') OR asset_type IS NULL),
    status     TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_conv_status ON conversations(status);
CREATE INDEX IF NOT EXISTS idx_conv_updated ON conversations(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_conv_asset ON conversations(asset_id) WHERE asset_id IS NOT NULL;

-- Messages
CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK(role IN ('user','assistant','system','tool')),
    content         TEXT NOT NULL DEFAULT '',
    content_type    TEXT NOT NULL DEFAULT 'text',
    metadata        JSON,
    model_id        TEXT,
    created_at      DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_msg_conv ON messages(conversation_id, created_at);

-- Tools
CREATE TABLE IF NOT EXISTS tools (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    display_name     TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    code             TEXT NOT NULL DEFAULT '',
    requirements     JSON NOT NULL DEFAULT '[]',
    parameters       JSON NOT NULL DEFAULT '[]',
    category         TEXT NOT NULL DEFAULT 'other',
    status           TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','tested','failed')),
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
CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_name ON tools(name);

-- Tool version history
CREATE TABLE IF NOT EXISTS tool_versions (
    id              TEXT PRIMARY KEY,
    tool_id         TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    code            TEXT NOT NULL,
    change_summary  TEXT NOT NULL DEFAULT '',
    created_at      DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tool_versions ON tool_versions(tool_id, version DESC);

-- Tool tags
CREATE TABLE IF NOT EXISTS tool_tags (
    tool_id TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    tag     TEXT NOT NULL,
    PRIMARY KEY (tool_id, tag)
);

-- Tool test history
CREATE TABLE IF NOT EXISTS tool_test_history (
    id          TEXT PRIMARY KEY,
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    passed      BOOLEAN NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    input_json  TEXT,
    output_json TEXT,
    error_msg   TEXT,
    created_at  DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_test_history ON tool_test_history(tool_id, created_at DESC);

-- Tool test cases (reusable parameter sets)
CREATE TABLE IF NOT EXISTS tool_test_cases (
    id          TEXT PRIMARY KEY,
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT 'Default',
    params_json TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME DEFAULT (datetime('now'))
);
