package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

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
SELECT id, name, proxy_url, enabled, health_status, consecutive_failures,
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
			&item.ID, &item.Name, &item.ProxyURL, &item.Enabled, &item.HealthStatus,
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
	_, err := r.db.ExecContext(ctx, `
INSERT INTO codex_turn_state_scans (
  account_id, model, status, attempt_count, last_proxy_id, last_proxy_url, last_state_length,
  last_error, last_attempt_at, last_success_at, next_attempt_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
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
  updated_at = NOW()`, scan.AccountID, strings.ToLower(strings.TrimSpace(scan.Model)), scan.Status,
		scan.AttemptCount, scan.LastProxyID, scan.LastProxyURL, scan.LastStateLength, scan.LastError,
		scan.LastAttemptAt, scan.LastSuccessAt, scan.NextAttemptAt)
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

func codexTurnStateAccountStatusCTE() string {
	return `
WITH eligible_accounts AS (
  SELECT id, name, type, COALESCE(credentials->>'plan_type', '') AS plan_type
  FROM accounts
  WHERE deleted_at IS NULL AND parent_account_id IS NULL AND platform = 'openai'
    AND type IN ('oauth', 'setup-token')
), account_models AS (
  SELECT id AS account_id, 'gpt-5.5'::varchar AS model FROM eligible_accounts
  UNION
  SELECT c.source_account_id, c.source_model
  FROM codex_turn_states c
  JOIN eligible_accounts a ON a.id = c.source_account_id
  WHERE c.source_model <> ''
  UNION
  SELECT sc.account_id, sc.model
  FROM codex_turn_state_scans sc
  JOIN eligible_accounts a ON a.id = sc.account_id
  WHERE sc.model <> ''
), status_rows AS (
  SELECT a.id AS account_id, a.name AS account_name, a.type AS account_type, a.plan_type,
         am.model,
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
  JOIN account_models am ON am.account_id = a.id
  LEFT JOIN LATERAL (
    SELECT value_length, issued_at, expires_at
    FROM codex_turn_states c
    WHERE c.source_account_id = a.id AND c.source_model = am.model
      AND c.value_length IN (292, 332) AND c.expires_at > NOW()
    ORDER BY c.value_length DESC, c.last_seen_at DESC, c.id DESC LIMIT 1
  ) ls ON TRUE
  LEFT JOIN codex_turn_state_scans sc ON sc.account_id = a.id AND sc.model = am.model
  LEFT JOIN codex_turn_state_proxies p ON p.id = sc.last_proxy_id
)
`
}

func (r *opsRepository) ListOpenAICodexTurnStateAccountStatuses(ctx context.Context, accountIDs []int64, page, pageSize int) (*service.OpenAICodexTurnStateAccountStatusList, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	var ids any
	if len(accountIDs) > 0 {
		ids = pq.Array(accountIDs)
	}
	cte := codexTurnStateAccountStatusCTE()
	var total int64
	if err := r.db.QueryRowContext(ctx, cte+`
SELECT COUNT(*) FROM status_rows WHERE ($1::bigint[] IS NULL OR account_id = ANY($1))`, ids).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, cte+`
SELECT account_id, account_name, account_type, plan_type, model, effective_status,
       COALESCE(state_length, 0), issued_at, expires_at, last_attempt_at, last_success_at,
       attempt_count, last_proxy_id, COALESCE(last_proxy_url, ''), last_error
FROM status_rows
WHERE ($1::bigint[] IS NULL OR account_id = ANY($1))
ORDER BY account_name ASC, account_id ASC, model ASC
LIMIT $2 OFFSET $3`, ids, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*service.OpenAICodexTurnStateAccountStatus, 0, pageSize)
	for rows.Next() {
		item := &service.OpenAICodexTurnStateAccountStatus{}
		if err := rows.Scan(
			&item.AccountID, &item.AccountName, &item.AccountType, &item.PlanType,
			&item.Model, &item.Status, &item.StateLength, &item.IssuedAt, &item.ExpiresAt,
			&item.LastAttemptAt, &item.LastSuccessAt, &item.AttemptCount, &item.LastProxyID,
			&item.LastProxyMasked, &item.LastError,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.OpenAICodexTurnStateAccountStatusList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *opsRepository) GetOpenAICodexTurnStateOperationsSummary(ctx context.Context) (*service.OpenAICodexTurnStateOperationsSummary, error) {
	summary := &service.OpenAICodexTurnStateOperationsSummary{}
	err := r.db.QueryRowContext(ctx, `
WITH eligible AS (
  SELECT id FROM accounts
  WHERE deleted_at IS NULL AND parent_account_id IS NULL AND platform = 'openai'
    AND type IN ('oauth', 'setup-token')
), ready AS (
  SELECT DISTINCT source_account_id FROM codex_turn_states
  WHERE value_length IN (292, 332) AND expires_at > NOW() AND source_account_id IS NOT NULL
)
SELECT (SELECT COUNT(*) FROM eligible),
       (SELECT COUNT(*) FROM eligible e JOIN ready r ON r.source_account_id = e.id),
       (SELECT COUNT(*) FROM eligible e LEFT JOIN ready r ON r.source_account_id = e.id WHERE r.source_account_id IS NULL),
       (SELECT COUNT(*) FROM codex_turn_state_scans WHERE status IN ('pending', 'running')),
       (SELECT COUNT(*) FROM codex_turn_state_proxies WHERE enabled = TRUE),
       (SELECT COUNT(*) FROM codex_turn_state_proxies WHERE enabled = TRUE AND health_status = 'healthy'),
       (SELECT COUNT(*) FROM proxies WHERE deleted_at IS NULL AND status = 'active'
          AND (expires_at IS NULL OR expires_at > NOW())),
       (SELECT MAX(last_attempt_at) FROM codex_turn_state_scans)`).Scan(
		&summary.OAuthAccounts, &summary.ReadyAccounts, &summary.MissingAccounts,
		&summary.RunningJobs, &summary.EnabledProxies, &summary.HealthyProxies, &summary.SharedProxies, &summary.LastScanAt,
	)
	return summary, err
}

func (r *opsRepository) ListObservedOpenAICodexTurnStateModels(ctx context.Context, accountID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT model
FROM (
  SELECT LOWER(BTRIM(source_model)) AS model, MAX(last_seen_at) AS seen_at
  FROM codex_turn_states
  WHERE source_account_id = $1 AND BTRIM(source_model) <> ''
  GROUP BY LOWER(BTRIM(source_model))
  UNION ALL
  SELECT LOWER(BTRIM(COALESCE(NULLIF(upstream_model, ''), model))) AS model,
         MAX(created_at) AS seen_at
  FROM usage_logs
  WHERE account_id = $1 AND BTRIM(COALESCE(NULLIF(upstream_model, ''), model)) <> ''
    AND created_at >= NOW() - INTERVAL '30 days'
  GROUP BY LOWER(BTRIM(COALESCE(NULLIF(upstream_model, ''), model)))
) observed
GROUP BY model
ORDER BY MAX(seen_at) DESC
LIMIT 12`, accountID)
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
