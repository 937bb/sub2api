-- 兼容历史数据库：143 迁移记录存在，但实际列可能因旧版 schema 漂移而缺失。
-- 仅补齐缺失列，不修改已有分组数据；可安全重复执行。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;
