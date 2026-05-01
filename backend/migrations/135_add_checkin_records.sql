-- 签到记录表
-- user_id + checkin_date 唯一索引防止重复签到
CREATE TABLE IF NOT EXISTS checkin_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_date DATE NOT NULL,                   -- 签到日期（用户本地日期）
    streak INT NOT NULL DEFAULT 1,                -- 连续签到天数（含当天）
    base_amount DECIMAL(20,8) NOT NULL DEFAULT 0, -- 基础奖励金额
    milestone_amount DECIMAL(20,8) NOT NULL DEFAULT 0, -- 里程碑奖励金额
    total_amount DECIMAL(20,8) NOT NULL DEFAULT 0, -- 总奖励金额 = base + milestone
    balance_type VARCHAR(20) NOT NULL DEFAULT 'permanent', -- 奖励余额类型
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 唯一索引：防刷
CREATE UNIQUE INDEX IF NOT EXISTS idx_checkin_records_user_date ON checkin_records(user_id, checkin_date);
-- 索引：用户维度查询
CREATE INDEX IF NOT EXISTS idx_checkin_records_user_id ON checkin_records(user_id);
-- 索引：按日期倒序查询
CREATE INDEX IF NOT EXISTS idx_checkin_records_date ON checkin_records(checkin_date DESC);

COMMENT ON TABLE checkin_records IS '每日签到记录表';
COMMENT ON COLUMN checkin_records.checkin_date IS '签到日期（防刷唯一索引依赖此字段）';
COMMENT ON COLUMN checkin_records.streak IS '连续签到天数（含当天，断签重置为1）';
COMMENT ON COLUMN checkin_records.base_amount IS '基础签到奖励';
COMMENT ON COLUMN checkin_records.milestone_amount IS '里程碑奖励（连续签到N天触发）';
COMMENT ON COLUMN checkin_records.total_amount IS '总奖励 = base_amount + milestone_amount';
COMMENT ON COLUMN checkin_records.balance_type IS '奖励余额类型：permanent / expirable';
