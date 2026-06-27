//go:build integration

package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIOAuthLegacyConfigMigration(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migrationPath := filepath.Join("..", "..", "migrations", "150_migrate_openai_oauth_legacy_config.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	require.NoError(t, err)

	var oauthPassthroughID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES (
	'openai-oauth-legacy-passthrough',
	'openai',
	'oauth',
	'{
		"openai_oauth_passthrough": true,
		"openai_passthrough": true,
		"openai_oauth_responses_websockets_v2_mode": "passthrough",
		"unrelated": "keep"
	}'::jsonb
)
RETURNING id`).Scan(&oauthPassthroughID))

	var oauthDisabledID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES (
	'openai-oauth-legacy-disabled',
	'openai',
	'oauth',
	'{
		"openai_oauth_passthrough": false,
		"openai_oauth_responses_websockets_v2_enabled": false
	}'::jsonb
)
RETURNING id`).Scan(&oauthDisabledID))

	var oauthEnabledID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES (
	'openai-oauth-legacy-enabled',
	'openai',
	'oauth',
	'{
		"openai_oauth_responses_websockets_v2_enabled": true
	}'::jsonb
)
RETURNING id`).Scan(&oauthEnabledID))

	var oauthGenericResponsesEnabledID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES (
	'openai-oauth-generic-responses-enabled',
	'openai',
	'oauth',
	'{
		"responses_websockets_v2_enabled": true
	}'::jsonb
)
RETURNING id`).Scan(&oauthGenericResponsesEnabledID))

	var oauthGenericOpenAIWSDisabledID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES (
	'openai-oauth-generic-openai-ws-disabled',
	'openai',
	'oauth',
	'{
		"openai_ws_enabled": false
	}'::jsonb
)
RETURNING id`).Scan(&oauthGenericOpenAIWSDisabledID))

	var setupTokenID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
	INSERT INTO accounts (name, platform, type, extra)
	VALUES (
		'openai-setup-token-legacy',
		'openai',
		'setup-token',
		'{
			"responses_websockets_v2_enabled": true,
			"openai_passthrough": true,
			"unrelated": "keep"
		}'::jsonb
	)
	RETURNING id`).Scan(&setupTokenID))

	var oauthWithAPIKeyOnlyWSID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
	INSERT INTO accounts (name, platform, type, extra)
	VALUES (
		'openai-oauth-stale-apikey-ws',
		'openai',
		'oauth',
		'{
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode": "passthrough",
			"unrelated": "keep"
		}'::jsonb
	)
	RETURNING id`).Scan(&oauthWithAPIKeyOnlyWSID))

	var apiKeyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES (
	'openai-apikey-passthrough',
	'openai',
	'apikey',
	'{
		"openai_passthrough": true,
		"openai_apikey_responses_websockets_v2_mode": "passthrough",
		"openai_oauth_responses_websockets_v2_enabled": true,
		"responses_websockets_v2_enabled": true,
		"openai_ws_enabled": false
	}'::jsonb
)
RETURNING id`).Scan(&apiKeyID))

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	passthroughExtra := mustQueryAccountExtraJSON(t, tx, ctx, oauthPassthroughID)
	require.Equal(t, "managed_session", gjson.Get(passthroughExtra, "openai_oauth_ws_mode").String())
	for _, field := range []string{
		"openai_oauth_passthrough",
		"openai_passthrough",
		"openai_oauth_responses_websockets_v2_mode",
		"openai_oauth_responses_websockets_v2_enabled",
	} {
		require.False(t, gjson.Get(passthroughExtra, field).Exists(), "%s should be removed from OAuth extra", field)
	}
	require.Equal(t, "keep", gjson.Get(passthroughExtra, "unrelated").String())

	disabledExtra := mustQueryAccountExtraJSON(t, tx, ctx, oauthDisabledID)
	require.Equal(t, "off", gjson.Get(disabledExtra, "openai_oauth_ws_mode").String())
	require.False(t, gjson.Get(disabledExtra, "openai_oauth_passthrough").Exists())
	require.False(t, gjson.Get(disabledExtra, "openai_oauth_responses_websockets_v2_enabled").Exists())

	enabledExtra := mustQueryAccountExtraJSON(t, tx, ctx, oauthEnabledID)
	require.Equal(t, "managed_session", gjson.Get(enabledExtra, "openai_oauth_ws_mode").String())
	require.False(t, gjson.Get(enabledExtra, "openai_oauth_responses_websockets_v2_enabled").Exists())

	genericResponsesEnabledExtra := mustQueryAccountExtraJSON(t, tx, ctx, oauthGenericResponsesEnabledID)
	require.Equal(t, "managed_session", gjson.Get(genericResponsesEnabledExtra, "openai_oauth_ws_mode").String())
	require.False(t, gjson.Get(genericResponsesEnabledExtra, "responses_websockets_v2_enabled").Exists())

	genericOpenAIWSDisabledExtra := mustQueryAccountExtraJSON(t, tx, ctx, oauthGenericOpenAIWSDisabledID)
	require.Equal(t, "off", gjson.Get(genericOpenAIWSDisabledExtra, "openai_oauth_ws_mode").String())
	require.False(t, gjson.Get(genericOpenAIWSDisabledExtra, "openai_ws_enabled").Exists())

	setupTokenExtra := mustQueryAccountExtraJSON(t, tx, ctx, setupTokenID)
	require.Equal(t, "managed_session", gjson.Get(setupTokenExtra, "openai_oauth_ws_mode").String())
	require.Equal(t, "keep", gjson.Get(setupTokenExtra, "unrelated").String())
	require.False(t, gjson.Get(setupTokenExtra, "responses_websockets_v2_enabled").Exists())
	require.False(t, gjson.Get(setupTokenExtra, "openai_passthrough").Exists())

	oauthWithAPIKeyOnlyWSExtra := mustQueryAccountExtraJSON(t, tx, ctx, oauthWithAPIKeyOnlyWSID)
	require.Equal(t, "keep", gjson.Get(oauthWithAPIKeyOnlyWSExtra, "unrelated").String())
	require.False(t, gjson.Get(oauthWithAPIKeyOnlyWSExtra, "openai_apikey_responses_websockets_v2_enabled").Exists())
	require.False(t, gjson.Get(oauthWithAPIKeyOnlyWSExtra, "openai_apikey_responses_websockets_v2_mode").Exists())
	require.False(t, gjson.Get(oauthWithAPIKeyOnlyWSExtra, "openai_oauth_ws_mode").Exists())

	apiKeyExtra := mustQueryAccountExtraJSON(t, tx, ctx, apiKeyID)
	require.True(t, gjson.Get(apiKeyExtra, "openai_passthrough").Bool())
	require.Equal(t, "passthrough", gjson.Get(apiKeyExtra, "openai_apikey_responses_websockets_v2_mode").String())
	require.True(t, gjson.Get(apiKeyExtra, "openai_oauth_responses_websockets_v2_enabled").Bool(), "migration must not touch APIKey extras")
	require.True(t, gjson.Get(apiKeyExtra, "responses_websockets_v2_enabled").Bool(), "migration must not touch APIKey generic extras")
	require.False(t, gjson.Get(apiKeyExtra, "openai_ws_enabled").Bool(), "migration must not touch APIKey generic extras")
	require.False(t, gjson.Get(apiKeyExtra, "openai_oauth_ws_mode").Exists())

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	require.JSONEq(t, passthroughExtra, mustQueryAccountExtraJSON(t, tx, ctx, oauthPassthroughID))
	require.JSONEq(t, disabledExtra, mustQueryAccountExtraJSON(t, tx, ctx, oauthDisabledID))
	require.JSONEq(t, enabledExtra, mustQueryAccountExtraJSON(t, tx, ctx, oauthEnabledID))
	require.JSONEq(t, genericResponsesEnabledExtra, mustQueryAccountExtraJSON(t, tx, ctx, oauthGenericResponsesEnabledID))
	require.JSONEq(t, genericOpenAIWSDisabledExtra, mustQueryAccountExtraJSON(t, tx, ctx, oauthGenericOpenAIWSDisabledID))
	require.JSONEq(t, setupTokenExtra, mustQueryAccountExtraJSON(t, tx, ctx, setupTokenID))
	require.JSONEq(t, oauthWithAPIKeyOnlyWSExtra, mustQueryAccountExtraJSON(t, tx, ctx, oauthWithAPIKeyOnlyWSID))
	require.JSONEq(t, apiKeyExtra, mustQueryAccountExtraJSON(t, tx, ctx, apiKeyID))

	for _, accountID := range []int64{oauthPassthroughID, oauthDisabledID, oauthEnabledID, oauthGenericResponsesEnabledID, oauthGenericOpenAIWSDisabledID, setupTokenID, oauthWithAPIKeyOnlyWSID} {
		var reportCount int
		require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'openai_oauth_legacy_config_migration'
  AND report_key LIKE $1 || ':%'
`, "account:"+strconv.FormatInt(accountID, 10)).Scan(&reportCount))
		require.Greater(t, reportCount, 0, "expected audit report for OAuth account %d", accountID)
	}

	var apiKeyReportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'openai_oauth_legacy_config_migration'
  AND report_key LIKE $1 || ':%'
`, "account:"+strconv.FormatInt(apiKeyID, 10)).Scan(&apiKeyReportCount))
	require.Zero(t, apiKeyReportCount, "APIKey accounts should not be migrated or reported")

	var reportDetails string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT details::text
FROM auth_identity_migration_reports
WHERE report_type = 'openai_oauth_legacy_config_migration'
  AND report_key = $1
`, "account:"+strconv.FormatInt(oauthPassthroughID, 10)+":openai_oauth_passthrough").Scan(&reportDetails))
	require.Equal(t, "boolean", gjson.Get(reportDetails, "old_value_class").String())
	require.Equal(t, "deleted", gjson.Get(reportDetails, "action").String())
	require.False(t, gjson.Get(reportDetails, "old_value").Exists(), "audit report must not store legacy values")
}

func TestOpenAIOAuthLegacyConfigMigration_RejectsConflictingNewMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{
			name: "oauth legacy enabled conflicts with existing off",
			extra: `{
				"openai_oauth_ws_mode": "off",
				"openai_oauth_responses_websockets_v2_enabled": true
			}`,
		},
		{
			name: "generic legacy enabled conflicts with existing off",
			extra: `{
				"openai_oauth_ws_mode": "off",
				"responses_websockets_v2_enabled": true
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := testTx(t)
			ctx := context.Background()

			migrationPath := filepath.Join("..", "..", "migrations", "150_migrate_openai_oauth_legacy_config.sql")
			migrationSQL, err := os.ReadFile(migrationPath)
			require.NoError(t, err)

			_, err = tx.ExecContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('openai-oauth-conflicting-ws-mode', 'openai', 'oauth', $1::jsonb)
`, tc.extra)
			require.NoError(t, err)

			_, err = tx.ExecContext(ctx, string(migrationSQL))
			require.Error(t, err)
			require.Contains(t, err.Error(), "conflicting OpenAI OAuth WS migration mode")
		})
	}
}

func TestOpenAIOAuthLegacyConfigMigration_IgnoresDeletedOAuthRows(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migrationPath := filepath.Join("..", "..", "migrations", "150_migrate_openai_oauth_legacy_config.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	require.NoError(t, err)

	var deletedID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type, extra, deleted_at)
VALUES (
	'openai-oauth-deleted-invalid-legacy',
	'openai',
	'oauth',
	'{"responses_websockets_v2_enabled": "true"}'::jsonb,
	NOW()
)
RETURNING id`).Scan(&deletedID))

	_, err = tx.ExecContext(ctx, string(migrationSQL))

	require.NoError(t, err)
	extra := mustQueryAccountExtraJSON(t, tx, ctx, deletedID)
	require.Equal(t, "true", gjson.Get(extra, "responses_websockets_v2_enabled").String())
	require.False(t, gjson.Get(extra, "openai_oauth_ws_mode").Exists())

	var reportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'openai_oauth_legacy_config_migration'
  AND report_key LIKE $1 || ':%'
`, "account:"+strconv.FormatInt(deletedID, 10)).Scan(&reportCount))
	require.Zero(t, reportCount, "deleted OAuth accounts should not be migrated or reported")
}

func TestOpenAIOAuthLegacyConfigMigration_RejectsWrongLegacyTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{
			name: "oauth legacy passthrough and mode wrong types",
			extra: `{
				"openai_oauth_passthrough": "true",
				"openai_oauth_responses_websockets_v2_mode": 123
			}`,
		},
		{
			name: "generic legacy ws wrong type",
			extra: `{
				"responses_websockets_v2_enabled": "true"
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := testTx(t)
			ctx := context.Background()

			migrationPath := filepath.Join("..", "..", "migrations", "150_migrate_openai_oauth_legacy_config.sql")
			migrationSQL, err := os.ReadFile(migrationPath)
			require.NoError(t, err)

			_, err = tx.ExecContext(ctx, `
INSERT INTO accounts (name, platform, type, extra)
VALUES ('openai-oauth-wrong-legacy-type', 'openai', 'oauth', $1::jsonb)
`, tc.extra)
			require.NoError(t, err)

			_, err = tx.ExecContext(ctx, string(migrationSQL))
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid OpenAI OAuth legacy config")
		})
	}
}

func mustQueryAccountExtraJSON(t *testing.T, tx *sql.Tx, ctx context.Context, accountID int64) string {
	t.Helper()

	var extra string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT extra::text
FROM accounts
WHERE id = $1
`, accountID).Scan(&extra))
	return extra
}
