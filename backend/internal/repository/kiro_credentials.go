package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Only persist the refreshed token if the account credentials and pinned route still match.
func (r *accountRepository) UpdateKiroOAuthCredentialsIfUnchanged(ctx context.Context, id int64, expected map[string]any, proxyID *int64, next map[string]any) (bool, error) {
	before, err := json.Marshal(normalizeJSONMap(expected))
	if err != nil {
		return false, err
	}
	after, err := json.Marshal(normalizeJSONMap(next))
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `WITH updated AS (
 UPDATE accounts SET credentials=$1::jsonb,updated_at=NOW()
 WHERE id=$2 AND platform='kiro' AND type='oauth' AND deleted_at IS NULL
 AND credentials=$3::jsonb AND proxy_id IS NOT DISTINCT FROM $4 RETURNING id)
 INSERT INTO scheduler_outbox(event_type,account_id,group_id,payload)
 SELECT $5,id,NULL,NULL FROM updated`, string(after), id, string(before), proxyID, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		r.syncSchedulerAccountSnapshotDetached(ctx, id)
	}
	return n > 0, nil
}
