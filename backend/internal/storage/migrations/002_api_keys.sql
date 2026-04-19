CREATE TABLE IF NOT EXISTS api_keys (
    id           TEXT PRIMARY KEY,
    provider     TEXT NOT NULL,
    display_name TEXT,
    key_enc      TEXT NOT NULL,
    base_url     TEXT,
    last_tested  DATETIME,
    test_status  TEXT,
    created_at   DATETIME DEFAULT (datetime('now')),
    updated_at   DATETIME DEFAULT (datetime('now'))
);
