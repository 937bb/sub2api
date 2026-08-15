-- 222_group_quota_bypass_concentrated_scheduling.sql
-- 将 Codex 超额绕过能力与集中填满式调度拆分为两个独立分组开关。

ALTER TABLE groups
ADD COLUMN IF NOT EXISTS quota_bypass_concentrated_scheduling_enabled BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.quota_bypass_concentrated_scheduling_enabled IS '是否对当前分组启用 Codex 超额绕过集中调度；仅在 quota_bypass_enabled 同时开启时生效';
