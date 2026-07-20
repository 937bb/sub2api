package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type growthRepository struct {
	db *sql.DB
}

const growthEligibleFundingValueSQL = `(
    COALESCE((
        SELECT SUM(GREATEST(po.amount - po.refund_amount, 0))
        FROM payment_orders po
        WHERE po.user_id = $1
          AND po.status IN ('PAID', 'RECHARGING', 'COMPLETED', 'PARTIALLY_REFUNDED')
    ), 0)
    + GREATEST(COALESCE((
        SELECT SUM(rc.value)
        FROM redeem_codes rc
        WHERE rc.used_by = $1
          AND rc.status = 'used'
          AND rc.type = 'admin_balance'
    ), 0), 0)
)`

const growthEligibleFundingSQL = `SELECT ` + growthEligibleFundingValueSQL + `::double precision`

type growthQueryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getGrowthEligibleFunding(ctx context.Context, queryer growthQueryRower, userID int64) (float64, error) {
	var amount float64
	if err := queryer.QueryRowContext(ctx, growthEligibleFundingSQL, userID).Scan(&amount); err != nil {
		return 0, err
	}
	return amount, nil
}

func NewGrowthRepository(db *sql.DB) service.GrowthRepository {
	return &growthRepository{db: db}
}

func (r *growthRepository) GetConfig(ctx context.Context) (*service.GrowthConfig, error) {
	const query = `
SELECT checkin_enabled, checkin_reward_mode,
       checkin_fixed_reward::double precision, checkin_min_reward::double precision,
       checkin_max_reward::double precision, checkin_streak_rewards,
       checkin_min_account_age_days, checkin_min_total_recharged::double precision,
       checkin_max_reward_paid_ratio::double precision,
       max_total_reward_paid_ratio::double precision,
       checkin_min_recent_spend::double precision,
       checkin_recent_spend_days, checkin_max_accounts_per_ip,
       checkin_max_accounts_per_device, leaderboard_enabled,
       leaderboard_anonymous, leaderboard_display_limit,
       leaderboard_reward_rules, updated_at
FROM growth_configs WHERE id = 1`
	var config service.GrowthConfig
	var streakJSON, rulesJSON []byte
	if err := r.db.QueryRowContext(ctx, query).Scan(
		&config.CheckinEnabled, &config.CheckinRewardMode,
		&config.CheckinFixedReward, &config.CheckinMinReward,
		&config.CheckinMaxReward, &streakJSON,
		&config.CheckinMinAccountAgeDays, &config.CheckinMinTotalRecharged,
		&config.CheckinMaxRewardPaidRatio,
		&config.MaxTotalRewardPaidRatio,
		&config.CheckinMinRecentSpend,
		&config.CheckinRecentSpendDays, &config.CheckinMaxAccountsPerIP,
		&config.CheckinMaxAccountsPerDevice, &config.LeaderboardEnabled,
		&config.LeaderboardAnonymous, &config.LeaderboardDisplayLimit,
		&rulesJSON, &config.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("get growth config: %w", err)
	}
	if err := json.Unmarshal(streakJSON, &config.CheckinStreakRewards); err != nil {
		return nil, fmt.Errorf("decode check-in streak rewards: %w", err)
	}
	if err := json.Unmarshal(rulesJSON, &config.LeaderboardRewardRules); err != nil {
		return nil, fmt.Errorf("decode leaderboard reward rules: %w", err)
	}
	return &config, nil
}

func (r *growthRepository) UpdateConfig(ctx context.Context, config service.GrowthConfig, updatedBy int64) (*service.GrowthConfig, error) {
	streakJSON, err := json.Marshal(config.CheckinStreakRewards)
	if err != nil {
		return nil, err
	}
	rulesJSON, err := json.Marshal(config.LeaderboardRewardRules)
	if err != nil {
		return nil, err
	}
	const query = `
UPDATE growth_configs SET
    checkin_enabled = $1, checkin_reward_mode = $2,
    checkin_fixed_reward = $3, checkin_min_reward = $4, checkin_max_reward = $5,
    checkin_streak_rewards = $6::jsonb, checkin_min_account_age_days = $7,
    checkin_min_total_recharged = $8, checkin_max_reward_paid_ratio = $9,
    max_total_reward_paid_ratio = $10, checkin_min_recent_spend = $11,
    checkin_recent_spend_days = $12, checkin_max_accounts_per_ip = $13,
    checkin_max_accounts_per_device = $14, leaderboard_enabled = $15,
    leaderboard_anonymous = $16, leaderboard_display_limit = $17,
    leaderboard_reward_rules = $18::jsonb,
    updated_by = $19, updated_at = NOW()
WHERE id = 1`
	if _, err := r.db.ExecContext(ctx, query,
		config.CheckinEnabled, config.CheckinRewardMode,
		config.CheckinFixedReward, config.CheckinMinReward, config.CheckinMaxReward,
		streakJSON, config.CheckinMinAccountAgeDays, config.CheckinMinTotalRecharged,
		config.CheckinMaxRewardPaidRatio,
		config.MaxTotalRewardPaidRatio,
		config.CheckinMinRecentSpend,
		config.CheckinRecentSpendDays, config.CheckinMaxAccountsPerIP,
		config.CheckinMaxAccountsPerDevice, config.LeaderboardEnabled,
		config.LeaderboardAnonymous, config.LeaderboardDisplayLimit,
		rulesJSON, updatedBy,
	); err != nil {
		return nil, fmt.Errorf("update growth config: %w", err)
	}
	return r.GetConfig(ctx)
}

func (r *growthRepository) GetCheckinStatus(ctx context.Context, userID int64, monthStart, monthEnd, today time.Time) (*service.GrowthCheckinStatus, error) {
	status := &service.GrowthCheckinStatus{MonthCheckins: []service.GrowthCheckin{}}
	const monthQuery = `
SELECT gc.checkin_date::text, gc.streak_days,
       gc.base_reward::double precision, gc.streak_reward::double precision,
       gc.total_reward::double precision, grl.balance_after::double precision
FROM growth_checkins gc
JOIN growth_reward_ledger grl ON grl.id = gc.ledger_id
WHERE gc.user_id = $1 AND gc.checkin_date >= $2::date AND gc.checkin_date < $3::date
ORDER BY gc.checkin_date`
	rows, err := r.db.QueryContext(ctx, monthQuery, userID, growthDate(monthStart), growthDate(monthEnd))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item service.GrowthCheckin
		if err := rows.Scan(&item.Date, &item.StreakDays, &item.BaseReward, &item.StreakReward, &item.TotalReward, &item.BalanceAfter); err != nil {
			return nil, err
		}
		status.MonthCheckins = append(status.MonthCheckins, item)
		if item.Date == today.Format("2006-01-02") {
			status.CheckedInToday = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	const summaryQuery = `
SELECT COUNT(*), COALESCE((
    SELECT streak_days FROM growth_checkins
    WHERE user_id = $1 AND checkin_date >= ($2::date - 1) AND checkin_date <= $2::date
    ORDER BY checkin_date DESC LIMIT 1
), 0)
FROM growth_checkins WHERE user_id = $1`
	if err := r.db.QueryRowContext(ctx, summaryQuery, userID, growthDate(today)).Scan(&status.TotalCheckins, &status.CurrentStreak); err != nil {
		return nil, err
	}
	return status, nil
}

func (r *growthRepository) ClaimCheckin(ctx context.Context, claim service.GrowthCheckinClaim) (*service.GrowthCheckin, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var createdAt time.Time
	var eligibleFunding float64
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT created_at, status FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, claim.UserID).Scan(&createdAt, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrUserNotFound
		}
		return nil, err
	}
	if status != service.StatusActive {
		return nil, r.denyClaim(ctx, tx, claim, "account_inactive", nil)
	}

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM growth_checkins WHERE user_id = $1 AND checkin_date = $2::date)`, claim.UserID, growthDate(claim.Date)).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, service.ErrGrowthAlreadyChecked
	}

	minimumCreatedAt := claim.Now.AddDate(0, 0, -claim.Config.CheckinMinAccountAgeDays)
	if createdAt.After(minimumCreatedAt) {
		return nil, r.denyClaim(ctx, tx, claim, "account_too_new", map[string]any{"minimum_age_days": claim.Config.CheckinMinAccountAgeDays})
	}
	eligibleFunding, err = getGrowthEligibleFunding(ctx, tx, claim.UserID)
	if err != nil {
		return nil, err
	}
	if eligibleFunding+1e-9 < claim.Config.CheckinMinTotalRecharged {
		return nil, r.denyClaim(ctx, tx, claim, "total_recharged_too_low", map[string]any{"eligible_funding": eligibleFunding, "required_funding": claim.Config.CheckinMinTotalRecharged})
	}

	var recentSpend float64
	spendSince := claim.Now.AddDate(0, 0, -claim.Config.CheckinRecentSpendDays)
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(actual_cost), 0)::double precision FROM usage_logs WHERE user_id = $1 AND created_at >= $2`, claim.UserID, spendSince).Scan(&recentSpend); err != nil {
		return nil, err
	}
	if recentSpend+1e-9 < claim.Config.CheckinMinRecentSpend {
		return nil, r.denyClaim(ctx, tx, claim, "recent_spend_too_low", map[string]any{"recent_spend": recentSpend, "required_spend": claim.Config.CheckinMinRecentSpend})
	}

	if err := r.checkClaimIdentityLimit(ctx, tx, claim, "ip_hash", claim.IPHash, claim.Config.CheckinMaxAccountsPerIP, "ip_account_limit"); err != nil {
		return nil, err
	}
	if err := r.checkClaimIdentityLimit(ctx, tx, claim, "device_hash", claim.DeviceHash, claim.Config.CheckinMaxAccountsPerDevice, "device_account_limit"); err != nil {
		return nil, err
	}

	streakDays := 1
	var previousDate sql.NullTime
	var previousStreak sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT checkin_date, streak_days FROM growth_checkins WHERE user_id = $1 ORDER BY checkin_date DESC LIMIT 1`, claim.UserID).Scan(&previousDate, &previousStreak); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if previousDate.Valid && previousStreak.Valid && sameDate(previousDate.Time, claim.Date.AddDate(0, 0, -1)) {
		streakDays = int(previousStreak.Int64) + 1
	}
	streakReward := 0.0
	for _, reward := range claim.Config.CheckinStreakRewards {
		if reward.Days == streakDays {
			streakReward += reward.Amount
		}
	}
	totalReward := roundGrowthAmount(claim.BaseReward + streakReward)
	if totalReward < 0 {
		return nil, fmt.Errorf("check-in reward must be non-negative")
	}
	var lifetimeCheckinReward, lifetimeGrowthReward float64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(SUM(amount) FILTER (WHERE source_type = 'checkin'), 0)::double precision,
       COALESCE(SUM(amount), 0)::double precision
FROM growth_reward_ledger WHERE user_id = $1`, claim.UserID).Scan(&lifetimeCheckinReward, &lifetimeGrowthReward); err != nil {
		return nil, err
	}
	maximumLifetimeReward := roundGrowthAmount(eligibleFunding * claim.Config.CheckinMaxRewardPaidRatio)
	if lifetimeCheckinReward+totalReward > maximumLifetimeReward+1e-9 {
		return nil, r.denyClaim(ctx, tx, claim, "lifetime_reward_cap", map[string]any{
			"lifetime_checkin_reward": lifetimeCheckinReward,
			"requested_reward":        totalReward,
			"maximum_reward":          maximumLifetimeReward,
			"eligible_funding":        eligibleFunding,
		})
	}
	maximumLifetimeGrowthReward := roundGrowthAmount(eligibleFunding * claim.Config.MaxTotalRewardPaidRatio)
	if lifetimeGrowthReward+totalReward > maximumLifetimeGrowthReward+1e-9 {
		return nil, r.denyClaim(ctx, tx, claim, "total_growth_reward_cap", map[string]any{
			"lifetime_growth_reward": lifetimeGrowthReward,
			"requested_reward":       totalReward,
			"maximum_reward":         maximumLifetimeGrowthReward,
			"eligible_funding":       eligibleFunding,
		})
	}

	var balanceAfter float64
	if err := tx.QueryRowContext(ctx, `UPDATE users SET balance = balance + $1, updated_at = NOW() WHERE id = $2 RETURNING balance::double precision`, totalReward, claim.UserID).Scan(&balanceAfter); err != nil {
		return nil, err
	}
	metadataJSON, _ := json.Marshal(map[string]any{"date": claim.Date.Format("2006-01-02"), "streak_days": streakDays, "base_reward": claim.BaseReward, "streak_reward": streakReward})
	var ledgerID int64
	sourceKey := fmt.Sprintf("%d:%s", claim.UserID, claim.Date.Format("2006-01-02"))
	if err := tx.QueryRowContext(ctx, `
INSERT INTO growth_reward_ledger (user_id, source_type, source_key, amount, balance_after, metadata)
VALUES ($1, 'checkin', $2, $3, $4, $5::jsonb) RETURNING id`, claim.UserID, sourceKey, totalReward, balanceAfter, metadataJSON).Scan(&ledgerID); err != nil {
		if isGrowthUniqueViolation(err) {
			return nil, service.ErrGrowthAlreadyChecked
		}
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO growth_checkins (user_id, checkin_date, streak_days, base_reward, streak_reward, total_reward, ip_hash, device_hash, ledger_id)
VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8, $9)`, claim.UserID, growthDate(claim.Date), streakDays, claim.BaseReward, streakReward, totalReward, claim.IPHash, claim.DeviceHash, ledgerID); err != nil {
		if isGrowthUniqueViolation(err) {
			return nil, service.ErrGrowthAlreadyChecked
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.GrowthCheckin{
		Date: claim.Date.Format("2006-01-02"), StreakDays: streakDays,
		BaseReward: claim.BaseReward, StreakReward: streakReward,
		TotalReward: totalReward, BalanceAfter: balanceAfter,
	}, nil
}

func (r *growthRepository) checkClaimIdentityLimit(ctx context.Context, tx *sql.Tx, claim service.GrowthCheckinClaim, column, hash string, limit int, reason string) error {
	if strings.TrimSpace(hash) == "" {
		return r.denyClaim(ctx, tx, claim, "missing_identity_signal", map[string]any{"signal": column})
	}
	query := fmt.Sprintf(`SELECT COUNT(DISTINCT user_id) FROM growth_checkins WHERE checkin_date = $1::date AND %s = $2 AND user_id <> $3`, column)
	var accounts int
	if err := tx.QueryRowContext(ctx, query, growthDate(claim.Date), hash, claim.UserID).Scan(&accounts); err != nil {
		return err
	}
	if accounts >= limit {
		return r.denyClaim(ctx, tx, claim, reason, map[string]any{"linked_accounts": accounts, "limit": limit})
	}
	return nil
}

func (r *growthRepository) denyClaim(ctx context.Context, tx *sql.Tx, claim service.GrowthCheckinClaim, reason string, evidence map[string]any) error {
	_ = tx.Rollback()
	r.recordRiskEvent(ctx, claim, "denied", reason, evidence)
	return growthClaimDeniedError(reason)
}

func growthClaimDeniedError(reason string) error {
	switch reason {
	case "account_inactive":
		return service.ErrGrowthAccountInactive
	case "account_too_new":
		return service.ErrGrowthAccountTooNew
	case "total_recharged_too_low":
		return service.ErrGrowthRechargeTooLow
	case "recent_spend_too_low":
		return service.ErrGrowthRecentSpendTooLow
	case "ip_account_limit", "device_account_limit", "missing_identity_signal":
		return service.ErrGrowthIdentityRisk
	case "lifetime_reward_cap":
		return service.ErrGrowthCheckinRewardCap
	case "total_growth_reward_cap":
		return service.ErrGrowthTotalRewardCap
	default:
		return service.ErrGrowthRewardIneligible
	}
}

func (r *growthRepository) recordRiskEvent(ctx context.Context, claim service.GrowthCheckinClaim, decision, reason string, evidence map[string]any) {
	if evidence == nil {
		evidence = map[string]any{}
	}
	encoded, _ := json.Marshal(evidence)
	_, _ = r.db.ExecContext(ctx, `
INSERT INTO growth_risk_events (user_id, event_type, decision, reason_code, ip_hash, device_hash, evidence)
VALUES ($1, 'checkin', $2, $3, $4, $5, $6::jsonb)`, claim.UserID, decision, reason, claim.IPHash, claim.DeviceHash, encoded)
}

func (r *growthRepository) GetLeaderboard(ctx context.Context, start, end time.Time, currentUserID int64, limit int, anonymous bool) (*service.GrowthLeaderboard, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	result := &service.GrowthLeaderboard{Items: []service.GrowthLeaderboardItem{}, Page: 1, PageSize: limit, Pages: 1}
	const query = `
WITH ranked AS (
    SELECT u.id AS user_id,
           COALESCE(
               NULLIF(BTRIM(u.username), ''),
               NULLIF(SPLIT_PART(BTRIM(u.email), '@', 1), ''),
               'User #' || u.id::text
           ) AS display_name,
           COALESCE(SUM(ul.actual_cost), 0)::double precision AS actual_cost,
           COUNT(ul.id) AS requests,
           ROW_NUMBER() OVER (ORDER BY COALESCE(SUM(ul.actual_cost), 0) DESC, u.id ASC) AS rank
    FROM users u
    JOIN usage_logs ul ON ul.user_id = u.id
    WHERE ul.created_at >= $1 AND ul.created_at < $2
      AND u.deleted_at IS NULL AND u.status = 'active'
    GROUP BY u.id, u.username, u.email
    HAVING SUM(ul.actual_cost) > 0
)
SELECT user_id, display_name, actual_cost, requests, rank
FROM ranked WHERE rank <= $3 OR user_id = $4
ORDER BY rank`
	rows, err := r.db.QueryContext(ctx, query, start, end, limit, currentUserID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item service.GrowthLeaderboardItem
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.ActualCost, &item.Requests, &item.Rank); err != nil {
			return nil, err
		}
		item.IsCurrentUser = item.UserID == currentUserID
		if anonymous && !item.IsCurrentUser {
			item.DisplayName = fmt.Sprintf("Anonymous #%d", item.Rank)
			item.UserID = 0
		}
		if item.Rank <= limit {
			result.Items = append(result.Items, item)
			result.Total++
		}
		if item.IsCurrentUser {
			copy := item
			result.CurrentUser = &copy
		}
	}
	return result, rows.Err()
}

func (r *growthRepository) SettleLeaderboard(ctx context.Context, period string, start, end time.Time, rules []service.GrowthLeaderboardRewardRule, maxTotalRewardPaidRatio float64) ([]int64, float64, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rulesJSON, _ := json.Marshal(map[string]any{
		"rules": rules, "max_total_reward_paid_ratio": maxTotalRewardPaidRatio,
		"reward_must_not_exceed_period_spend": true,
	})
	var settlementID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO growth_leaderboard_settlements (period_type, period_start, period_end, rule_snapshot)
VALUES ($1, $2, $3, $4::jsonb)
ON CONFLICT (period_type, period_start, period_end) DO NOTHING
RETURNING id`, period, start, end, rulesJSON).Scan(&settlementID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	maxRank := 0
	for _, rule := range rules {
		if rule.RankEnd > maxRank {
			maxRank = rule.RankEnd
		}
	}
	rows, err := tx.QueryContext(ctx, `
SELECT user_id, actual_cost, rank FROM (
    SELECT ul.user_id, SUM(ul.actual_cost)::double precision AS actual_cost,
           ROW_NUMBER() OVER (ORDER BY SUM(ul.actual_cost) DESC, ul.user_id ASC) AS rank
    FROM usage_logs ul JOIN users u ON u.id = ul.user_id
    WHERE ul.created_at >= $1 AND ul.created_at < $2
      AND u.deleted_at IS NULL AND u.status = 'active'
    GROUP BY ul.user_id
    HAVING SUM(ul.actual_cost) > 0
) ranked WHERE rank <= $3 ORDER BY rank`, start, end, maxRank)
	if err != nil {
		return nil, 0, err
	}
	type rankedUser struct {
		userID int64
		spend  float64
		rank   int
	}
	users := make([]rankedUser, 0, maxRank)
	for rows.Next() {
		var item rankedUser
		if err := rows.Scan(&item.userID, &item.spend, &item.rank); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		users = append(users, item)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	total := 0.0
	rewardedUserIDs := make([]int64, 0, len(users))
	rewardedUsers := make(map[int64]struct{}, len(users))
	for _, ranked := range users {
		for _, rule := range rules {
			if ranked.rank < rule.RankStart || ranked.rank > rule.RankEnd {
				continue
			}
			var eligibleFunding, lifetimeGrowthReward float64
			if err := tx.QueryRowContext(ctx, `SELECT `+growthEligibleFundingValueSQL+`::double precision,
       COALESCE((
           SELECT SUM(grl.amount) FROM growth_reward_ledger grl WHERE grl.user_id = u.id
       ), 0)::double precision
FROM users u
WHERE u.id = $1 AND u.deleted_at IS NULL AND u.status = 'active'
FOR UPDATE`, ranked.userID).Scan(&eligibleFunding, &lifetimeGrowthReward); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				return nil, 0, err
			}
			allowed, maximumLifetimeReward := growthLeaderboardRewardAllowed(
				ranked.spend, eligibleFunding, lifetimeGrowthReward,
				rule.RewardAmount, maxTotalRewardPaidRatio,
			)
			if !allowed {
				continue
			}
			var balanceAfter float64
			if err := tx.QueryRowContext(ctx, `UPDATE users SET balance = balance + $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL RETURNING balance::double precision`, rule.RewardAmount, ranked.userID).Scan(&balanceAfter); err != nil {
				return nil, 0, err
			}
			metadata, _ := json.Marshal(map[string]any{"period": period, "period_start": start, "period_end": end, "rank": ranked.rank, "spend": ranked.spend, "eligible_funding": eligibleFunding, "max_lifetime_growth_reward": maximumLifetimeReward, "rule_id": rule.ID, "settlement_id": settlementID})
			sourceKey := fmt.Sprintf("%s:%s:%s:%d", period, start.Format(time.RFC3339), rule.ID, ranked.userID)
			if _, err := tx.ExecContext(ctx, `INSERT INTO growth_reward_ledger (user_id, source_type, source_key, amount, balance_after, metadata) VALUES ($1, 'leaderboard', $2, $3, $4, $5::jsonb)`, ranked.userID, sourceKey, rule.RewardAmount, balanceAfter, metadata); err != nil {
				return nil, 0, err
			}
			if _, exists := rewardedUsers[ranked.userID]; !exists {
				rewardedUsers[ranked.userID] = struct{}{}
				rewardedUserIDs = append(rewardedUserIDs, ranked.userID)
			}
			total += rule.RewardAmount
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE growth_leaderboard_settlements SET rewarded_users = $1, total_reward = $2 WHERE id = $3`, len(rewardedUserIDs), total, settlementID); err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return rewardedUserIDs, total, nil
}

func (r *growthRepository) ListRewardLedger(ctx context.Context, page, pageSize int) ([]service.GrowthRewardLedgerItem, int64, error) {
	page, pageSize = normalizeGrowthPagination(page, pageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM growth_reward_ledger`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT l.id, l.user_id, COALESCE(u.email, ''), l.source_type, l.source_key,
       l.amount::double precision, l.balance_after::double precision, l.metadata, l.created_at
FROM growth_reward_ledger l LEFT JOIN users u ON u.id = l.user_id
ORDER BY l.id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.GrowthRewardLedgerItem, 0, pageSize)
	for rows.Next() {
		var item service.GrowthRewardLedgerItem
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.UserID, &item.Email, &item.SourceType, &item.SourceKey, &item.Amount, &item.BalanceAfter, &metadata, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *growthRepository) ListRiskEvents(ctx context.Context, page, pageSize int) ([]service.GrowthRiskEvent, int64, error) {
	page, pageSize = normalizeGrowthPagination(page, pageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM growth_risk_events`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT e.id, e.user_id, COALESCE(u.email, ''), e.decision, e.reason_code, e.evidence, e.created_at
FROM growth_risk_events e LEFT JOIN users u ON u.id = e.user_id
ORDER BY e.id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.GrowthRiskEvent, 0, pageSize)
	for rows.Next() {
		var item service.GrowthRiskEvent
		var userID sql.NullInt64
		var evidence []byte
		if err := rows.Scan(&item.ID, &userID, &item.Email, &item.Decision, &item.ReasonCode, &evidence, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		if userID.Valid {
			value := userID.Int64
			item.UserID = &value
		}
		_ = json.Unmarshal(evidence, &item.Evidence)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func roundGrowthAmount(value float64) float64 {
	return float64(int64(value*100+0.5)) / 100
}

func growthDate(value time.Time) string { return value.Format("2006-01-02") }

func growthLeaderboardRewardAllowed(periodSpend, eligibleFunding, lifetimeGrowthReward, rewardAmount, maxPaidRatio float64) (bool, float64) {
	maximumLifetimeReward := roundGrowthAmount(eligibleFunding * maxPaidRatio)
	if rewardAmount <= 0 || rewardAmount > periodSpend+1e-9 {
		return false, maximumLifetimeReward
	}
	return lifetimeGrowthReward+rewardAmount <= maximumLifetimeReward+1e-9, maximumLifetimeReward
}

func normalizeGrowthPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func isGrowthUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate key value")
}
