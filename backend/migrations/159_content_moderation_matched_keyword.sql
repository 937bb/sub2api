-- Record the configured rule text that caused a keyword block for admin audit logs.

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS matched_keyword VARCHAR(255) NOT NULL DEFAULT '';
