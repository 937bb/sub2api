package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *accountRepository) RepairLegacyOpenAISiwc(ctx context.Context, expected *service.Account, credentials map[string]any) (bool, error) {
	if r == nil || r.sql == nil || !expected.IsOpenAISiwc() || strings.EqualFold(expected.GetCredential("auth_mode"), "siwc") {
		return false, errors.New("SIWC legacy account required")
	}
	payload, err := json.Marshal(credentials)
	if err != nil {
		return false, err
	}
	previous, err := json.Marshal(expected.Credentials)
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `
	 WITH updated AS (
	  UPDATE accounts SET credentials = credentials || $1::jsonb, updated_at = NOW()
	  WHERE id = $2 AND deleted_at IS NULL AND platform = 'openai' AND type = 'oauth'
	  AND credentials = $3::jsonb AND proxy_id IS NOT DISTINCT FROM $4
	  RETURNING id
	 )
	 INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
	 SELECT $5, id, NULL, NULL FROM updated`, string(payload), expected.ID, string(previous), expected.ProxyID, service.SchedulerOutboxEventAccountChanged)
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
