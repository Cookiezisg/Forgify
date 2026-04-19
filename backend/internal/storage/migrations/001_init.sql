-- App configuration key-value store
CREATE TABLE IF NOT EXISTS app_config (
    key        TEXT PRIMARY KEY,
    value      TEXT,
    updated_at DATETIME DEFAULT (datetime('now'))
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

-- Messages
CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK(role IN ('user','assistant','system')),
    content         TEXT NOT NULL,
    token_count     INTEGER,
    metadata        JSON,
    created_at      DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_messages_conv ON messages(conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_conversations_updated ON conversations(updated_at DESC);
