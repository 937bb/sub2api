package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// UpdateOpenAISiwcModels preserves independently rotating credentials. The
// directory and scheduler invalidation commit in the same SQL statement.
func (r *accountRepository) UpdateOpenAISiwcModels(ctx context.Context, id int64, subject, clientID string, models []string) (bool, error) {
	if models == nil {
		models = []string{}
	}
	payload, err := json.Marshal(models)
	if err != nil {
		return false, err
	}
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		 UPDATE accounts SET credentials = jsonb_set(credentials, '{siwc_models}', $1::jsonb), updated_at = NOW()
		 WHERE id = $2 AND deleted_at IS NULL AND platform = 'openai' AND type = 'oauth'
		 AND lower(credentials->>'auth_mode') = 'siwc'
		 AND credentials->>'subject' = $3 AND credentials->>'client_id' = $4
		 RETURNING id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $5, id, NULL, NULL FROM updated`, string(payload), id, subject, clientID, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

// Compare the grant used by the upstream call, but allow concurrent directory
// and administrative configuration updates. Merge only the supplied token fields.
func (r *accountRepository) UpdateOpenAISiwcCredentials(ctx context.Context, expected *service.Account, credentials map[string]any) (bool, error) {
	if r == nil || r.sql == nil || !expected.IsOpenAISiwc() {
		return false, errors.New("SIWC credential repository unavailable")
	}
	payload, err := json.Marshal(credentials)
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		 UPDATE accounts SET credentials = credentials || $1::jsonb, updated_at = NOW()
		 WHERE id = $2 AND deleted_at IS NULL AND platform = 'openai' AND type = 'oauth'
		 AND lower(credentials->>'auth_mode') = 'siwc'
		 AND credentials->>'subject' = $3 AND credentials->>'client_id' = $4
		 AND credentials->>'ext_agent_host_id' = $5
		 AND credentials->>'access_token' = $6
		 AND COALESCE(credentials->>'refresh_token', '') = $7
		 AND proxy_id IS NOT DISTINCT FROM $8
		 RETURNING id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $9, id, NULL, NULL FROM updated`, string(payload), expected.ID, expected.GetCredential("subject"), expected.GetCredential("client_id"), expected.GetCredential("ext_agent_host_id"), expected.GetCredential("access_token"), expected.GetCredential("refresh_token"), expected.ProxyID, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, expected.ID)
	return true, nil
}
