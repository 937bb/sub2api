-- 余额明细表：区分永久余额和有效期余额
-- 每笔余额变动（充值/扣费/奖励/过期/管理员调整）都记录为一条 entry
-- user.balance 字段保留为缓存汇总值，通过触发器或应用层同步
CREATE TABLE IF NOT EXISTS balance_entries (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount DECIMAL(20,8) NOT NULL,              -- 该笔变动金额（正=入账，负=扣减）
    remaining DECIMAL(20,8) NOT NULL DEFAULT 0, -- 当前剩余可用金额（仅入账 entry 有意义，扣减 entry 为 0）
    balance_type VARCHAR(20) NOT NULL DEFAULT 'permanent', -- 'permanent' | 'expirable'
    source VARCHAR(40) NOT NULL,                -- 来源类型
    note TEXT NOT NULL DEFAULT '',               -- 描述/备注
    expires_at TIMESTAMPTZ NULL,                 -- 过期时间（balance_type='expirable' 时必填）
    expired BOOLEAN NOT NULL DEFAULT FALSE,      -- 是否已被过期清理
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 索引：用户维度查询
CREATE INDEX IF NOT EXISTS idx_balance_entries_user_id ON balance_entries(user_id);
-- 索引：过期扫描（定时任务用）
CREATE INDEX IF NOT EXISTS idx_balance_entries_expiry ON balance_entries(expires_at)
    WHERE balance_type = 'expirable' AND expired = FALSE AND remaining > 0;
-- 索引：扣减时按过期时间排序（先扣即将过期的）
CREATE INDEX IF NOT EXISTS idx_balance_entries_deduct_order ON balance_entries(user_id, expires_at ASC NULLS LAST)
    WHERE remaining > 0 AND expired = FALSE;

COMMENT ON TABLE balance_entries IS '余额明细表，记录每笔余额变动';
COMMENT ON COLUMN balance_entries.amount IS '变动金额（正=入账，负=扣减记录）';
COMMENT ON COLUMN balance_entries.remaining IS '该笔入账余额剩余可用金额';
COMMENT ON COLUMN balance_entries.balance_type IS '余额类型：permanent=永久，expirable=有有效期';
COMMENT ON COLUMN balance_entries.source IS '来源：recharge/redeem/checkin/leaderboard/admin/promo/affiliate/first_redeem_bonus/redeem_bonus/cashback/oauth_grant/consumption/expiry_clear';
COMMENT ON COLUMN balance_entries.expires_at IS '过期时间，仅 expirable 类型使用';
COMMENT ON COLUMN balance_entries.expired IS '是否已被过期清理任务标记过期';
