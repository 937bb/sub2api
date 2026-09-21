package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *opsRepository) ListReusableOpenAICodexTurnStateProxies(ctx context.Context) ([]*service.OpenAICodexTurnStateProxy, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, protocol, host, port, COALESCE(username, ''), COALESCE(password, '')
FROM proxies
WHERE deleted_at IS NULL AND status = 'active'
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpenAICodexTurnStateProxy, 0)
	for rows.Next() {
		var id int64
		var name, protocol, host, username, password string
		var port int
		if err := rows.Scan(&id, &name, &protocol, &host, &port, &username, &password); err != nil {
			return nil, err
		}
		proxyURL := &url.URL{Scheme: strings.ToLower(strings.TrimSpace(protocol)), Host: net.JoinHostPort(host, strconv.Itoa(port))}
		if username != "" || password != "" {
			proxyURL.User = url.UserPassword(username, password)
		}
		items = append(items, &service.OpenAICodexTurnStateProxy{
			Source: "shared", SourceID: id, Name: name, ProxyURL: proxyURL.String(), Enabled: true,
		})
	}
	return items, rows.Err()
}

func (r *opsRepository) ListOpenAICodexTurnStateProxies(ctx context.Context, enabledOnly bool) ([]*service.OpenAICodexTurnStateProxy, error) {
	where := ""
	if enabledOnly {
		where = "WHERE enabled = TRUE"
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, proxy_url, enabled, route_binding_enabled, health_status, consecutive_failures,
       last_checked_at, last_success_at, last_error, created_at, updated_at
FROM codex_turn_state_proxies `+where+`
ORDER BY enabled DESC, COALESCE(last_success_at, 'epoch'::timestamptz) ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpenAICodexTurnStateProxy, 0)
	for rows.Next() {
		item := &service.OpenAICodexTurnStateProxy{}
		if err := rows.Scan(
			&item.ID, &item.Name, &item.ProxyURL, &item.Enabled, &item.RouteBindingEnabled, &item.HealthStatus,
			&item.ConsecutiveFailures, &item.LastCheckedAt, &item.LastSuccessAt,
			&item.LastError, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Source = "state"
		item.SourceID = item.ID
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *opsRepository) CreateOpenAICodexTurnStateProxies(ctx context.Context, proxies []*service.OpenAICodexTurnStateProxy) (int, error) {
	if len(proxies) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	added := 0
	for _, proxy := range proxies {
		if proxy == nil || strings.TrimSpace(proxy.ProxyURL) == "" {
			continue
		}
		result, err := tx.ExecContext(ctx, `
INSERT INTO codex_turn_state_proxies (name, proxy_url, enabled)
VALUES ($1, $2, TRUE)
ON CONFLICT (proxy_url) DO UPDATE SET
  name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE codex_turn_state_proxies.name END,
  enabled = TRUE,
  updated_at = NOW()`, strings.TrimSpace(proxy.Name), strings.TrimSpace(proxy.ProxyURL))
		if err != nil {
			return 0, err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			added++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return added, nil
}

func (r *opsRepository) SetOpenAICodexTurnStateProxyEnabled(ctx context.Context, id int64, enabled bool) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE codex_turn_state_proxies SET enabled = $2, updated_at = NOW() WHERE id = $1`, id, enabled)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *opsRepository) SetOpenAICodexTurnStateProxyRouteBinding(ctx context.Context, id int64, enabled bool) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE codex_turn_state_proxies
SET route_binding_enabled = $2, updated_at = NOW()
WHERE id = $1`, id, enabled)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *opsRepository) DeleteOpenAICodexTurnStateProxy(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM codex_turn_state_proxies WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *opsRepository) UpdateOpenAICodexTurnStateProxyHealth(ctx context.Context, proxy *service.OpenAICodexTurnStateProxy) error {
	if proxy == nil || proxy.ID <= 0 {
		return fmt.Errorf("invalid Codex turn-state proxy")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE codex_turn_state_proxies
SET health_status = $2, consecutive_failures = $3, last_checked_at = $4,
    last_success_at = $5, last_error = $6, updated_at = NOW()
WHERE id = $1`, proxy.ID, proxy.HealthStatus, proxy.ConsecutiveFailures,
		proxy.LastCheckedAt, proxy.LastSuccessAt, proxy.LastError)
	return err
}

func (r *opsRepository) UpsertOpenAICodexTurnStateScan(ctx context.Context, scan *service.OpenAICodexTurnStateScan) error {
	if scan == nil || scan.AccountID <= 0 || strings.TrimSpace(scan.Model) == "" {
		return fmt.Errorf("invalid Codex turn-state scan")
	}
	result, err := r.db.ExecContext(ctx, `
INSERT INTO codex_turn_state_scans (
  account_id, model, status, attempt_count, last_proxy_id, last_proxy_url, last_state_length,
  last_error, last_attempt_at, last_success_at, next_attempt_at
) SELECT $1::bigint,$2::varchar(255),$3::varchar(20),$4::integer,$5::bigint,
         $6::text,$7::integer,$8::text,$9::timestamptz,$10::timestamptz,$11::timestamptz
WHERE $12::text = '' OR EXISTS (
  SELECT 1 FROM codex_turn_state_scans owned
  WHERE owned.account_id = $1 AND owned.model = $2
    AND owned.lease_id = $12 AND owned.lease_until > NOW()
)
ON CONFLICT (account_id, model) DO UPDATE SET
  status = EXCLUDED.status,
  attempt_count = EXCLUDED.attempt_count,
  last_proxy_id = EXCLUDED.last_proxy_id,
  last_proxy_url = EXCLUDED.last_proxy_url,
  last_state_length = EXCLUDED.last_state_length,
  last_error = EXCLUDED.last_error,
  last_attempt_at = EXCLUDED.last_attempt_at,
  last_success_at = EXCLUDED.last_success_at,
  next_attempt_at = EXCLUDED.next_attempt_at,
  updated_at = NOW()
WHERE ($12 <> '' AND codex_turn_state_scans.lease_id = $12 AND codex_turn_state_scans.lease_until > NOW())
   OR ($12 = '' AND (codex_turn_state_scans.lease_id IS NULL
       OR codex_turn_state_scans.lease_until IS NULL OR codex_turn_state_scans.lease_until <= NOW()))`, scan.AccountID, strings.ToLower(strings.TrimSpace(scan.Model)), scan.Status,
		scan.AttemptCount, scan.LastProxyID, scan.LastProxyURL, scan.LastStateLength, scan.LastError,
		scan.LastAttemptAt, scan.LastSuccessAt, scan.NextAttemptAt, strings.TrimSpace(scan.LeaseID))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("codex turn-state scan lease lost or held by another worker")
	}
	return nil
}

// ClaimOpenAICodexTurnStateScan atomically claims an account/model scan row.
// This is intentionally separate from the scan status: a ready or retrying
// row can still be leased while it is being refreshed, and a lease owner is
// the only instance allowed to release it.
func (r *opsRepository) ClaimOpenAICodexTurnStateScan(ctx context.Context, accountID int64, model, leaseID string, leaseUntil time.Time) (bool, error) {
	if accountID <= 0 || strings.TrimSpace(model) == "" || strings.TrimSpace(leaseID) == "" || leaseUntil.IsZero() {
		return false, fmt.Errorf("invalid Codex turn-state scan lease")
	}
	var claimedAccountID int64
	err := r.db.QueryRowContext(ctx, `
INSERT INTO codex_turn_state_scans (account_id, model, status, lease_id, lease_until)
VALUES ($1, LOWER(BTRIM($2)), 'pending', $3, $4)
ON CONFLICT (account_id, model) DO UPDATE SET
  lease_id = EXCLUDED.lease_id,
  lease_until = EXCLUDED.lease_until,
  updated_at = NOW()
WHERE codex_turn_state_scans.lease_id IS NULL
   OR codex_turn_state_scans.lease_until IS NULL
   OR codex_turn_state_scans.lease_until <= NOW()
   OR codex_turn_state_scans.lease_id = EXCLUDED.lease_id
RETURNING account_id`, accountID, model, strings.TrimSpace(leaseID), leaseUntil).Scan(&claimedAccountID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return claimedAccountID == accountID, nil
}

func (r *opsRepository) ReleaseOpenAICodexTurnStateScan(ctx context.Context, accountID int64, model, leaseID string) error {
	if accountID <= 0 || strings.TrimSpace(model) == "" || strings.TrimSpace(leaseID) == "" {
		return fmt.Errorf("invalid Codex turn-state scan lease")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE codex_turn_state_scans
SET lease_id = NULL, lease_until = NULL, updated_at = NOW()
WHERE account_id = $1 AND model = LOWER(BTRIM($2)) AND lease_id = $3`,
		accountID, model, strings.TrimSpace(leaseID))
	return err
}

func (r *opsRepository) GetOpenAICodexTurnStateScan(ctx context.Context, accountID int64, model string) (*service.OpenAICodexTurnStateScan, error) {
	item := &service.OpenAICodexTurnStateScan{}
	err := r.db.QueryRowContext(ctx, `
SELECT account_id, model, status, attempt_count, last_proxy_id, last_proxy_url, last_state_length,
       last_error, last_attempt_at, last_success_at, next_attempt_at, updated_at
FROM codex_turn_state_scans WHERE account_id = $1 AND model = $2`, accountID, strings.ToLower(strings.TrimSpace(model))).Scan(
		&item.AccountID, &item.Model, &item.Status, &item.AttemptCount, &item.LastProxyID, &item.LastProxyURL,
		&item.LastStateLength, &item.LastError, &item.LastAttemptAt, &item.LastSuccessAt,
		&item.NextAttemptAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (r *opsRepository) ListRecentlyUsedOpenAICodexAccountIDs(ctx context.Context, usedSince time.Time) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id
FROM accounts
WHERE deleted_at IS NULL AND parent_account_id IS NULL
  AND platform = 'openai' AND type IN ('oauth', 'setup-token')
  AND status = 'active' AND schedulable IS TRUE AND last_used_at >= $1
  AND (expires_at IS NULL OR expires_at > NOW())
  AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW())
  AND (overload_until IS NULL OR overload_until <= NOW())
  AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at <= NOW())
ORDER BY last_used_at DESC, id ASC`, usedSince)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func codexTurnStateAccountStatusCTE() string {
	return `
WITH eligible_accounts AS (
  SELECT id, name, type,
         CASE
           WHEN raw_plan.value IN ('pro', 'pro5x', 'pro20x', 'pro_5x', 'pro_20x', 'pro-5x', 'pro-20x',
                                   'pro_lite', 'pro-lite', 'prolite', 'chatgpt_pro', 'chatgptpro') THEN 'pro'
           WHEN raw_plan.value IN ('team', 'business', 'chatgpt_team', 'self_serve_business', 'self_serve_business_usage_based',
                                   'self_serve_business_prolite', 'selfservebusinessprolite') THEN 'team'
           WHEN raw_plan.value = '' AND extra->'team_oauth_verified' = 'true'::jsonb
             AND COALESCE(BTRIM(extra->>'team_oauth_verified_workspace_id'), '') <> ''
             AND BTRIM(extra->>'team_oauth_verified_workspace_id') = BTRIM(credentials->>'chatgpt_account_id') THEN 'team'
           ELSE raw_plan.value
         END AS plan_type
  FROM accounts
  CROSS JOIN LATERAL (
    SELECT LOWER(BTRIM(COALESCE(NULLIF(BTRIM(credentials->>'plan_type'), ''),
                               NULLIF(BTRIM(credentials->>'chatgpt_plan_type'), ''),
                               NULLIF(BTRIM(credentials->>'subscription_plan'), ''), ''))) AS value
  ) raw_plan
  WHERE deleted_at IS NULL AND parent_account_id IS NULL AND platform = 'openai'
    AND type IN ('oauth', 'setup-token')
    AND ($2::bigint[] IS NULL OR id = ANY($2))
    AND (
      $2::bigint[] IS NOT NULL
      OR (
        status = 'active' AND schedulable IS TRUE AND last_used_at >= $1
        AND (expires_at IS NULL OR expires_at > NOW())
        AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW())
        AND (overload_until IS NULL OR overload_until <= NOW())
        AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at <= NOW())
      )
    )
), scan_policy AS (
  SELECT $4::jsonb AS value
), scan_rules AS (
  SELECT rule->>'plan_type' AS plan_type, rule->>'model' AS model,
         ARRAY(SELECT value::integer FROM jsonb_array_elements_text(rule->'target_lengths')) AS target_lengths,
         position
  FROM scan_policy
  CROSS JOIN LATERAL jsonb_array_elements(COALESCE(NULLIF(value->'rules', 'null'::jsonb), '[]'::jsonb)) WITH ORDINALITY AS rules(rule, position)
), target_models AS (
  SELECT DISTINCT LOWER(BTRIM(value)) AS model
  FROM UNNEST($3::text[]) AS value
  WHERE BTRIM(value) <> ''
), account_models AS (
  SELECT a.id AS account_id, tm.model
  FROM eligible_accounts a
  CROSS JOIN target_models tm
  UNION
  SELECT u.account_id, LOWER(BTRIM(COALESCE(NULLIF(u.upstream_model, ''), u.model))) AS model
  FROM usage_logs u
  JOIN eligible_accounts a ON a.id = u.account_id
  WHERE u.created_at >= $1
    AND (LOWER(BTRIM(COALESCE(NULLIF(u.upstream_model, ''), u.model))) LIKE 'gpt-5%'
      OR LOWER(BTRIM(COALESCE(NULLIF(u.upstream_model, ''), u.model))) LIKE 'gpt-6%')
  GROUP BY u.account_id, LOWER(BTRIM(COALESCE(NULLIF(u.upstream_model, ''), u.model)))
  UNION
  SELECT c.source_account_id, c.source_model
  FROM codex_turn_states c
  JOIN eligible_accounts a ON a.id = c.source_account_id
  WHERE c.last_seen_at >= $1 AND (c.source_model LIKE 'gpt-5%' OR c.source_model LIKE 'gpt-6%')
  UNION
  SELECT sc.account_id, sc.model
  FROM codex_turn_state_scans sc
  JOIN eligible_accounts a ON a.id = sc.account_id
  WHERE sc.updated_at >= $1 AND (sc.model LIKE 'gpt-5%' OR sc.model LIKE 'gpt-6%')
), account_targets AS (
  SELECT am.account_id, am.model,
         COALESCE(matched.target_lengths,
                  ARRAY(SELECT value::integer FROM jsonb_array_elements_text(policy.value->'target_lengths'))) AS target_lengths
  FROM account_models am
  JOIN eligible_accounts a ON a.id = am.account_id
  CROSS JOIN scan_policy policy
  LEFT JOIN LATERAL (
    SELECT rules.target_lengths
    FROM scan_rules rules
    WHERE rules.plan_type IN (a.plan_type, '*') AND rules.model IN (am.model, '*')
    ORDER BY (rules.plan_type <> '*') DESC, (rules.model <> '*') DESC, rules.position
    LIMIT 1
  ) matched ON TRUE
), status_rows AS (
  SELECT a.id AS account_id, a.name AS account_name, a.type AS account_type, a.plan_type,
         am.model, am.target_lengths,
         ls.value_length AS state_length, ls.issued_at, ls.expires_at,
         sc.status AS scan_status, COALESCE(sc.attempt_count, 0) AS attempt_count,
         sc.last_proxy_id, sc.last_attempt_at, sc.last_success_at, COALESCE(sc.last_error, '') AS last_error,
         COALESCE(NULLIF(sc.last_proxy_url, ''), p.proxy_url, '') AS last_proxy_url,
         CASE
           WHEN ls.expires_at > NOW() + INTERVAL '5 minutes' THEN 'ready'
           WHEN ls.expires_at > NOW() THEN 'expiring'
           WHEN sc.status IN ('pending', 'running', 'retry_wait', 'failed', 'disabled') THEN sc.status
           ELSE 'missing'
         END AS effective_status
  FROM eligible_accounts a
  JOIN account_targets am ON am.account_id = a.id
  LEFT JOIN LATERAL (
    SELECT value_length, issued_at, expires_at
    FROM codex_turn_states c
    WHERE c.source_account_id = a.id AND c.source_model = am.model
      AND c.value_length = ANY(am.target_lengths) AND c.expires_at > NOW()
      AND c.issued_at <= NOW() AND c.issued_at + INTERVAL '1 hour' > NOW()
    ORDER BY array_position(am.target_lengths, c.value_length), c.expires_at DESC, c.last_seen_at DESC, c.state_hash DESC LIMIT 1
  ) ls ON TRUE
  LEFT JOIN codex_turn_state_scans sc ON sc.account_id = a.id AND sc.model = am.model
  LEFT JOIN codex_turn_state_proxies p ON p.id = sc.last_proxy_id
)
`
}

func codexTurnStatePolicyJSON(settings *service.OpenAICodexTurnStateScanSettings) (string, error) {
	if settings == nil {
		settings = (*service.OpsService)(nil).GetOpenAICodexTurnStateScanSettings()
	}
	policy := struct {
		TargetLengths []int                                    `json:"target_lengths"`
		Rules         []service.OpenAICodexTurnStateLengthRule `json:"rules"`
	}{TargetLengths: settings.TargetLengths, Rules: settings.Rules}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("encode Codex state policy: %w", err)
	}
	return string(encoded), nil
}

func (r *opsRepository) ListOpenAICodexTurnStateAccountStatuses(ctx context.Context, accountIDs []int64, targetModels []string, settings *service.OpenAICodexTurnStateScanSettings, page, pageSize int) (*service.OpenAICodexTurnStateAccountStatusList, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	var ids any
	if len(accountIDs) > 0 {
		ids = pq.Array(accountIDs)
	}
	models := pq.Array(targetModels)
	policy, err := codexTurnStatePolicyJSON(settings)
	if err != nil {
		return nil, err
	}
	cte := codexTurnStateAccountStatusCTE()
	usedSince := time.Now().Add(-time.Hour)
	var total int64
	if err := r.db.QueryRowContext(ctx, cte+`
SELECT COUNT(*) FROM status_rows WHERE ($2::bigint[] IS NULL OR account_id = ANY($2))`, usedSince, ids, models, policy).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, cte+`
SELECT account_id, account_name, account_type, plan_type, model, target_lengths, effective_status,
       COALESCE(state_length, 0), issued_at, expires_at, last_attempt_at, last_success_at,
       attempt_count, last_proxy_id, COALESCE(last_proxy_url, ''), last_error
FROM status_rows
WHERE ($2::bigint[] IS NULL OR account_id = ANY($2))
ORDER BY account_name ASC, account_id ASC, model ASC
LIMIT $5 OFFSET $6`, usedSince, ids, models, policy, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*service.OpenAICodexTurnStateAccountStatus, 0, pageSize)
	for rows.Next() {
		item := &service.OpenAICodexTurnStateAccountStatus{}
		var targetLengths pq.Int64Array
		if err := rows.Scan(
			&item.AccountID, &item.AccountName, &item.AccountType, &item.PlanType,
			&item.Model, &targetLengths, &item.Status, &item.StateLength, &item.IssuedAt, &item.ExpiresAt,
			&item.LastAttemptAt, &item.LastSuccessAt, &item.AttemptCount, &item.LastProxyID,
			&item.LastProxyMasked, &item.LastError,
		); err != nil {
			return nil, err
		}
		item.TargetLengths = make([]int, len(targetLengths))
		for i, length := range targetLengths {
			item.TargetLengths[i] = int(length)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.OpenAICodexTurnStateAccountStatusList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *opsRepository) GetOpenAICodexTurnStateOperationsSummary(ctx context.Context, targetModels []string, settings *service.OpenAICodexTurnStateScanSettings) (*service.OpenAICodexTurnStateOperationsSummary, error) {
	summary := &service.OpenAICodexTurnStateOperationsSummary{}
	policy, err := codexTurnStatePolicyJSON(settings)
	if err != nil {
		return nil, err
	}
	err = r.db.QueryRowContext(ctx, codexTurnStateAccountStatusCTE()+`, account_readiness AS (
  SELECT account_id,
         COUNT(*) AS target_count,
         COUNT(*) FILTER (WHERE effective_status = 'ready') AS ready_count
  FROM status_rows
  GROUP BY account_id
)
SELECT (SELECT COUNT(*) FROM eligible_accounts),
       (SELECT COUNT(*) FROM account_readiness WHERE target_count > 0 AND ready_count = target_count),
       (SELECT COUNT(*) FROM account_readiness WHERE ready_count < target_count),
       (SELECT COUNT(*) FROM status_rows WHERE effective_status = 'ready'),
       (SELECT COUNT(*) FROM status_rows),
       (SELECT COUNT(*) FROM status_rows WHERE scan_status IN ('pending', 'running')),
       (SELECT COUNT(*) FROM codex_turn_state_proxies WHERE enabled = TRUE),
       (SELECT COUNT(*) FROM codex_turn_state_proxies WHERE enabled = TRUE AND health_status = 'healthy'),
       (SELECT COUNT(*) FROM proxies WHERE deleted_at IS NULL AND status = 'active'
          AND (expires_at IS NULL OR expires_at > NOW())),
       (SELECT MAX(sc.last_attempt_at) FROM codex_turn_state_scans sc JOIN eligible_accounts e ON e.id = sc.account_id)`,
		time.Now().Add(-time.Hour), nil, pq.Array(targetModels), policy).Scan(
		&summary.OAuthAccounts, &summary.ReadyAccounts, &summary.MissingAccounts,
		&summary.ReadyModelSlots, &summary.TotalModelSlots, &summary.RunningJobs, &summary.EnabledProxies, &summary.HealthyProxies, &summary.SharedProxies, &summary.LastScanAt,
	)
	return summary, err
}

func (r *opsRepository) ListObservedOpenAICodexTurnStateModels(ctx context.Context, accountID int64, usedSince time.Time) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT model
FROM (
  SELECT LOWER(BTRIM(source_model)) AS model, MAX(last_seen_at) AS seen_at
  FROM codex_turn_states
  WHERE source_account_id = $1 AND BTRIM(source_model) <> '' AND last_seen_at >= $2
  GROUP BY LOWER(BTRIM(source_model))
  UNION ALL
  SELECT LOWER(BTRIM(COALESCE(NULLIF(upstream_model, ''), model))) AS model,
         MAX(created_at) AS seen_at
  FROM usage_logs
  WHERE account_id = $1 AND BTRIM(COALESCE(NULLIF(upstream_model, ''), model)) <> ''
    AND created_at >= $2
  GROUP BY LOWER(BTRIM(COALESCE(NULLIF(upstream_model, ''), model)))
) observed
GROUP BY model
ORDER BY MAX(seen_at) DESC
LIMIT 12`, accountID, usedSince)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	models := make([]string, 0, 4)
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, rows.Err()
}
