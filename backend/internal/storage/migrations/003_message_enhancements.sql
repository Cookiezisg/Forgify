-- Add content_type and model_id to messages for card-type rendering and model tracking
ALTER TABLE messages ADD COLUMN content_type TEXT NOT NULL DEFAULT 'text';
ALTER TABLE messages ADD COLUMN model_id TEXT;

-- Add indexes for conversation queries
CREATE INDEX IF NOT EXISTS idx_conversations_status ON conversations(status);
CREATE INDEX IF NOT EXISTS idx_conversations_asset ON conversations(asset_id) WHERE asset_id IS NOT NULL;
