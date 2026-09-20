package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// This test executes the complete policy queries against PostgreSQL. Temporary
// tables and a pg_temp-only search path keep every write isolated from real data.
func TestCodexTurnStatePlanModelPolicyPostgres(t *testing.T) {
	dsn := os.Getenv("SUB2API_STATE_TEST_DSN")
	if dsn == "" {
		t.Skip("set SUB2API_STATE_TEST_DSN to an isolated PostgreSQL test database")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	_, err = db.ExecContext(ctx, `SET search_path TO pg_temp;
CREATE TEMP TABLE accounts (
  id BIGINT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'oauth',
  credentials JSONB NOT NULL, extra JSONB, deleted_at TIMESTAMPTZ, parent_account_id BIGINT,
  platform TEXT NOT NULL DEFAULT 'openai', status TEXT NOT NULL DEFAULT 'active',
  schedulable BOOLEAN NOT NULL DEFAULT TRUE, last_used_at TIMESTAMPTZ DEFAULT NOW(),
  expires_at TIMESTAMPTZ, temp_unschedulable_until TIMESTAMPTZ,
  overload_until TIMESTAMPTZ, rate_limit_reset_at TIMESTAMPTZ
);
CREATE TEMP TABLE usage_logs (account_id BIGINT, upstream_model TEXT, model TEXT, created_at TIMESTAMPTZ);
CREATE TEMP TABLE codex_turn_states (
  state_hash TEXT PRIMARY KEY, source_account_id BIGINT NOT NULL, source_model TEXT NOT NULL,
  value_length INT NOT NULL, issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 minutes',
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TEMP TABLE codex_turn_state_scans (
  account_id BIGINT NOT NULL, model TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
  attempt_count INT, last_proxy_id BIGINT, last_proxy_url TEXT, last_attempt_at TIMESTAMPTZ,
  last_success_at TIMESTAMPTZ, last_error TEXT, updated_at TIMESTAMPTZ DEFAULT NOW(),
  PRIMARY KEY(account_id, model)
);
CREATE TEMP TABLE codex_turn_state_proxies (id BIGINT PRIMARY KEY, proxy_url TEXT, enabled BOOLEAN, health_status TEXT);
CREATE TEMP TABLE proxies (id BIGINT PRIMARY KEY, deleted_at TIMESTAMPTZ, status TEXT, expires_at TIMESTAMPTZ);
INSERT INTO accounts(id,name,credentials) VALUES
  (1, 'Pro', '{"plan_type":"Prolite"}'),
  (2, 'Team', '{"plan_type":" ","chatgpt_plan_type":"Self_serve_business_prolite"}'),
  (3, 'Plus', '{"subscription_plan":"plus"}'),
  (4, 'Inactive', '{"plan_type":"pro"}');
UPDATE accounts SET status = 'disabled' WHERE id = 4;
INSERT INTO codex_turn_states(state_hash,source_account_id,source_model,value_length) VALUES
  ('pro-terra-292', 1, 'gpt-5.6-terra', 292),
  ('pro-terra-wrong-332', 1, 'gpt-5.6-terra', 332),
  ('pro-astra-292', 1, 'gpt-6-astra', 292),
  ('team-terra-286', 2, 'gpt-5.6-terra', 286),
  ('team-terra-wrong-292', 2, 'gpt-5.6-terra', 292),
  ('team-astra-273', 2, 'gpt-6-astra', 273),
  ('plus-terra-332', 3, 'gpt-5.6-terra', 332),
  ('plus-astra-292', 3, 'gpt-6-astra', 292);`)
	require.NoError(t, err)
	repo := &opsRepository{db: db}
	settings := &service.OpenAICodexTurnStateScanSettings{
		TargetLengths: []int{332, 292},
		Rules: []service.OpenAICodexTurnStateLengthRule{
			{PlanType: "pro", Model: "*", TargetLengths: []int{292}},
			{PlanType: "team", Model: "gpt-5.6-terra", TargetLengths: []int{286}},
			{PlanType: "team", Model: "gpt-6-astra", TargetLengths: []int{273}},
		},
	}
	models := []string{"gpt-5.6-terra", "gpt-6-astra"}
	list, err := repo.ListOpenAICodexTurnStateAccountStatuses(ctx, nil, models, settings, 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 6, list.Total)
	require.Len(t, list.Items, 6)
	rows := make(map[int64]map[string]*service.OpenAICodexTurnStateAccountStatus)
	for _, item := range list.Items {
		if rows[item.AccountID] == nil {
			rows[item.AccountID] = make(map[string]*service.OpenAICodexTurnStateAccountStatus)
		}
		rows[item.AccountID][item.Model] = item
		require.Equal(t, "ready", item.Status)
	}
	require.Equal(t, []int{292}, rows[1][models[0]].TargetLengths)
	require.Equal(t, 292, rows[1][models[0]].StateLength)
	require.Equal(t, []int{286}, rows[2][models[0]].TargetLengths)
	require.Equal(t, 286, rows[2][models[0]].StateLength)
	require.Equal(t, "team", rows[2][models[0]].PlanType)
	require.Equal(t, []int{273}, rows[2][models[1]].TargetLengths)
	require.Equal(t, 273, rows[2][models[1]].StateLength)
	require.Equal(t, []int{332, 292}, rows[3][models[0]].TargetLengths)
	summary, err := repo.GetOpenAICodexTurnStateOperationsSummary(ctx, models, settings)
	require.NoError(t, err)
	require.EqualValues(t, 3, summary.OAuthAccounts)
	require.EqualValues(t, 3, summary.ReadyAccounts)
	require.EqualValues(t, 6, summary.ReadyModelSlots)
	require.EqualValues(t, 6, summary.TotalModelSlots)

	// Updating the policy changes both detail and aggregate readiness immediately.
	settings.Rules[1].TargetLengths = []int{300}
	list, err = repo.ListOpenAICodexTurnStateAccountStatuses(ctx, []int64{2}, models, settings, 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
	require.Equal(t, []int{300}, list.Items[0].TargetLengths)
	require.Equal(t, "missing", list.Items[0].Status)
	require.Equal(t, 0, list.Items[0].StateLength)
	summary, err = repo.GetOpenAICodexTurnStateOperationsSummary(ctx, models, settings)
	require.NoError(t, err)
	require.EqualValues(t, 2, summary.ReadyAccounts)
	require.EqualValues(t, 1, summary.MissingAccounts)
	require.EqualValues(t, 5, summary.ReadyModelSlots)

	// Plan-wide rules outrank model-only rules; an exact rule outranks both.
	settings.Rules = append(settings.Rules,
		service.OpenAICodexTurnStateLengthRule{PlanType: "*", Model: "gpt-5.6-terra", TargetLengths: []int{286}},
	)
	list, err = repo.ListOpenAICodexTurnStateAccountStatuses(ctx, []int64{1}, models, settings, 1, 20)
	require.NoError(t, err)
	require.Equal(t, []int{292}, list.Items[0].TargetLengths)
	require.Equal(t, 292, list.Items[0].StateLength)
	settings.Rules = append(settings.Rules,
		service.OpenAICodexTurnStateLengthRule{PlanType: "pro", Model: "gpt-5.6-terra", TargetLengths: []int{332}},
	)
	list, err = repo.ListOpenAICodexTurnStateAccountStatuses(ctx, []int64{1, 3}, models, settings, 1, 20)
	require.NoError(t, err)
	for _, item := range list.Items {
		if item.Model != models[0] {
			continue
		}
		if item.AccountID == 1 {
			require.Equal(t, []int{332}, item.TargetLengths)
			require.Equal(t, 332, item.StateLength)
		} else {
			require.Equal(t, []int{286}, item.TargetLengths)
			require.Equal(t, "missing", item.Status)
		}
	}

	// Null rules are valid for an explicit fallback-only policy.
	settings.Rules = nil
	_, err = repo.GetOpenAICodexTurnStateOperationsSummary(ctx, models, settings)
	require.NoError(t, err)

	// Verified Team metadata is a fallback only when every explicit plan is blank.
	_, err = db.ExecContext(ctx, `INSERT INTO accounts(id,name,credentials,extra) VALUES
  (5, 'Verified Team', '{"plan_type":" ","chatgpt_plan_type":"","chatgpt_account_id":" workspace-a "}', '{"team_oauth_verified":true,"team_oauth_verified_workspace_id":"workspace-a"}'),
  (6, 'Mismatched Team', '{"chatgpt_account_id":"workspace-b"}', '{"team_oauth_verified":true,"team_oauth_verified_workspace_id":"workspace-a"}'),
  (7, 'Explicit Pro', '{"subscription_plan":"pro","chatgpt_account_id":"workspace-a"}', '{"team_oauth_verified":true,"team_oauth_verified_workspace_id":"workspace-a"}'),
  (8, 'Explicit Unknown', '{"plan_type":" Future_Plan ","chatgpt_account_id":"workspace-a"}', '{"team_oauth_verified":true,"team_oauth_verified_workspace_id":"workspace-a"}'),
  (9, 'Unverified String', '{"chatgpt_account_id":"workspace-a"}', '{"team_oauth_verified":"true","team_oauth_verified_workspace_id":"workspace-a"}'),
  (10, 'Empty Workspace', '{"chatgpt_account_id":" "}', '{"team_oauth_verified":true,"team_oauth_verified_workspace_id":" "}');`)
	require.NoError(t, err)
	settings.Rules = []service.OpenAICodexTurnStateLengthRule{
		{PlanType: "pro", Model: "*", TargetLengths: []int{292}},
		{PlanType: "team", Model: "gpt-5.6-terra", TargetLengths: []int{286}},
		{PlanType: "team", Model: "gpt-6-astra", TargetLengths: []int{273}},
	}
	list, err = repo.ListOpenAICodexTurnStateAccountStatuses(ctx, []int64{5, 6, 7, 8, 9, 10}, []string{models[0]}, settings, 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 6)
	for _, item := range list.Items {
		switch item.AccountID {
		case 5:
			require.Equal(t, "team", item.PlanType)
			require.Equal(t, []int{286}, item.TargetLengths)
		case 7:
			require.Equal(t, "pro", item.PlanType)
			require.Equal(t, []int{292}, item.TargetLengths)
		case 8:
			require.Equal(t, "future_plan", item.PlanType)
			require.Equal(t, []int{332, 292}, item.TargetLengths)
		default:
			require.Empty(t, item.PlanType)
			require.Equal(t, []int{332, 292}, item.TargetLengths)
		}
	}
}
