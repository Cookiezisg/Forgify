-- Pending code change (for review-before-apply flow)
ALTER TABLE tools ADD COLUMN pending_code TEXT;
ALTER TABLE tools ADD COLUMN pending_summary TEXT;
