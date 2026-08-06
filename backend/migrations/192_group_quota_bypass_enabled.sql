-- 192_group_quota_bypass_enabled.sql
-- 添加分组级别的 OpenAI Codex 超额绕过开关

ALTER TABLE groups
ADD COLUMN IF NOT EXISTS quota_bypass_enabled BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.quota_bypass_enabled IS '是否为该分组下符合条件的 OpenAI OAuth 账号启用 Codex 超额绕过，不区分 Plus、Team 或其他订阅方案';
