-- Migrate legacy OpenAI OAuth passthrough/WS configuration into OAuth-only adapter names.
-- APIKey passthrough configuration is intentionally untouched.
DO $$
DECLARE
    invalid_config RECORD;
BEGIN
    WITH invalid_rows AS (
        SELECT
            a.id,
            checks.key AS field_name,
            checks.expected AS expected_type,
            COALESCE(jsonb_typeof(a.extra -> checks.key), 'missing') AS actual_type
        FROM accounts AS a
        CROSS JOIN (VALUES
            ('openai_oauth_passthrough', 'boolean'),
            ('openai_passthrough', 'boolean'),
            ('openai_oauth_responses_websockets_v2_mode', 'string'),
            ('openai_oauth_responses_websockets_v2_enabled', 'boolean'),
            ('responses_websockets_v2_enabled', 'boolean'),
            ('openai_ws_enabled', 'boolean')
        ) AS checks(key, expected)
        WHERE a.platform = 'openai'
          AND a.type IN ('oauth', 'setup-token')
          AND a.deleted_at IS NULL
          AND a.extra ? checks.key
          AND jsonb_typeof(a.extra -> checks.key) <> checks.expected

        UNION ALL

        SELECT
            a.id,
            'openai_oauth_responses_websockets_v2_mode' AS field_name,
            'off/ctx_pool/shared/dedicated/passthrough/managed_session' AS expected_type,
            'invalid_string' AS actual_type
        FROM accounts AS a
        WHERE a.platform = 'openai'
          AND a.type IN ('oauth', 'setup-token')
          AND a.deleted_at IS NULL
          AND a.extra ? 'openai_oauth_responses_websockets_v2_mode'
          AND jsonb_typeof(a.extra -> 'openai_oauth_responses_websockets_v2_mode') = 'string'
          AND LOWER(BTRIM(a.extra ->> 'openai_oauth_responses_websockets_v2_mode')) NOT IN (
              'off',
              'ctx_pool',
              'shared',
              'dedicated',
              'passthrough',
              'managed_session'
          )

        UNION ALL

        SELECT
            a.id,
            'openai_oauth_ws_mode' AS field_name,
            'off/managed_session' AS expected_type,
            CASE
                WHEN jsonb_typeof(a.extra -> 'openai_oauth_ws_mode') = 'string' THEN 'invalid_string'
                ELSE COALESCE(jsonb_typeof(a.extra -> 'openai_oauth_ws_mode'), 'missing')
            END AS actual_type
        FROM accounts AS a
        WHERE a.platform = 'openai'
          AND a.type IN ('oauth', 'setup-token')
          AND a.deleted_at IS NULL
          AND (
              a.extra ? 'openai_oauth_responses_websockets_v2_mode'
              OR a.extra ? 'openai_oauth_responses_websockets_v2_enabled'
              OR a.extra ? 'responses_websockets_v2_enabled'
              OR a.extra ? 'openai_ws_enabled'
          )
          AND a.extra ? 'openai_oauth_ws_mode'
          AND (
              jsonb_typeof(a.extra -> 'openai_oauth_ws_mode') <> 'string'
              OR LOWER(BTRIM(a.extra ->> 'openai_oauth_ws_mode')) NOT IN ('off', 'managed_session')
          )
    )
    SELECT * INTO invalid_config
    FROM invalid_rows
    ORDER BY id, field_name
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'invalid OpenAI OAuth legacy config: account %, field %, expected %, got %',
            invalid_config.id,
            invalid_config.field_name,
            invalid_config.expected_type,
            invalid_config.actual_type;
    END IF;
END $$;

DO $$
DECLARE
    conflict_config RECORD;
BEGIN
    WITH ws_targets AS (
        SELECT
            a.id,
            COALESCE(
                CASE
                    WHEN a.extra ? 'openai_oauth_responses_websockets_v2_mode' THEN
                        CASE LOWER(BTRIM(a.extra ->> 'openai_oauth_responses_websockets_v2_mode'))
                            WHEN 'off' THEN 'off'
                            WHEN 'ctx_pool' THEN 'managed_session'
                            WHEN 'shared' THEN 'managed_session'
                            WHEN 'dedicated' THEN 'managed_session'
                            WHEN 'passthrough' THEN 'managed_session'
                            WHEN 'managed_session' THEN 'managed_session'
                        END
                END,
                CASE
                    WHEN a.extra ? 'openai_oauth_responses_websockets_v2_enabled' THEN
                        CASE WHEN (a.extra ->> 'openai_oauth_responses_websockets_v2_enabled')::boolean THEN 'managed_session' ELSE 'off' END
                    WHEN a.extra ? 'responses_websockets_v2_enabled' THEN
                        CASE WHEN (a.extra ->> 'responses_websockets_v2_enabled')::boolean THEN 'managed_session' ELSE 'off' END
                    WHEN a.extra ? 'openai_ws_enabled' THEN
                        CASE WHEN (a.extra ->> 'openai_ws_enabled')::boolean THEN 'managed_session' ELSE 'off' END
                END
            ) AS target_mode,
            CASE
                WHEN a.extra ? 'openai_oauth_ws_mode' THEN LOWER(BTRIM(a.extra ->> 'openai_oauth_ws_mode'))
            END AS existing_target
        FROM accounts AS a
        WHERE a.platform = 'openai'
          AND a.type IN ('oauth', 'setup-token')
          AND a.deleted_at IS NULL
          AND (
              a.extra ? 'openai_oauth_responses_websockets_v2_mode'
              OR a.extra ? 'openai_oauth_responses_websockets_v2_enabled'
              OR a.extra ? 'responses_websockets_v2_enabled'
              OR a.extra ? 'openai_ws_enabled'
          )
    )
    SELECT * INTO conflict_config
    FROM ws_targets
    WHERE existing_target IS NOT NULL
      AND target_mode IS NOT NULL
      AND existing_target <> target_mode
    ORDER BY id
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'conflicting OpenAI OAuth WS migration mode: account %, existing %, target %',
            conflict_config.id,
            conflict_config.existing_target,
            conflict_config.target_mode;
    END IF;
END $$;

WITH legacy_field_reports AS (
    SELECT
        a.id AS account_id,
        a.type AS account_type,
        legacy.key AS old_key,
        jsonb_typeof(a.extra -> legacy.key) AS old_value_class,
        CASE
            WHEN legacy.key = 'openai_oauth_responses_websockets_v2_mode' THEN 'migrated'
            WHEN legacy.key = 'openai_oauth_responses_websockets_v2_enabled'
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_mode') THEN 'migrated'
            WHEN legacy.key = 'responses_websockets_v2_enabled'
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_mode')
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_enabled') THEN 'migrated'
            WHEN legacy.key = 'openai_ws_enabled'
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_mode')
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_enabled')
                 AND NOT (a.extra ? 'responses_websockets_v2_enabled') THEN 'migrated'
            ELSE 'deleted'
        END AS action,
        CASE
            WHEN legacy.key = 'openai_oauth_responses_websockets_v2_mode' THEN 'openai_oauth_ws_mode'
            WHEN legacy.key = 'openai_oauth_responses_websockets_v2_enabled'
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_mode') THEN 'openai_oauth_ws_mode'
            WHEN legacy.key = 'responses_websockets_v2_enabled'
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_mode')
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_enabled') THEN 'openai_oauth_ws_mode'
            WHEN legacy.key = 'openai_ws_enabled'
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_mode')
                 AND NOT (a.extra ? 'openai_oauth_responses_websockets_v2_enabled')
                 AND NOT (a.extra ? 'responses_websockets_v2_enabled') THEN 'openai_oauth_ws_mode'
        END AS new_key,
        CASE legacy.key
            WHEN 'openai_oauth_responses_websockets_v2_mode' THEN
                CASE LOWER(BTRIM(a.extra ->> legacy.key))
                    WHEN 'off' THEN 'off'
                    WHEN 'ctx_pool' THEN 'managed_session'
                    WHEN 'shared' THEN 'managed_session'
                    WHEN 'dedicated' THEN 'managed_session'
                    WHEN 'passthrough' THEN 'managed_session'
                    WHEN 'managed_session' THEN 'managed_session'
                END
            WHEN 'openai_oauth_responses_websockets_v2_enabled' THEN
                CASE
                    WHEN a.extra ? 'openai_oauth_responses_websockets_v2_mode' THEN NULL
                    WHEN (a.extra ->> legacy.key)::boolean THEN 'managed_session'
                    ELSE 'off'
                END
            WHEN 'responses_websockets_v2_enabled' THEN
                CASE
                    WHEN a.extra ? 'openai_oauth_responses_websockets_v2_mode'
                         OR a.extra ? 'openai_oauth_responses_websockets_v2_enabled' THEN NULL
                    WHEN (a.extra ->> legacy.key)::boolean THEN 'managed_session'
                    ELSE 'off'
                END
            WHEN 'openai_ws_enabled' THEN
                CASE
                    WHEN a.extra ? 'openai_oauth_responses_websockets_v2_mode'
                         OR a.extra ? 'openai_oauth_responses_websockets_v2_enabled'
                         OR a.extra ? 'responses_websockets_v2_enabled' THEN NULL
                    WHEN (a.extra ->> legacy.key)::boolean THEN 'managed_session'
                    ELSE 'off'
                END
        END AS new_value,
        CASE
            WHEN legacy.key = 'openai_oauth_responses_websockets_v2_enabled'
                 AND a.extra ? 'openai_oauth_responses_websockets_v2_mode'
                THEN 'ignored_by_openai_oauth_responses_websockets_v2_mode'
            WHEN legacy.key = 'responses_websockets_v2_enabled'
                 AND a.extra ? 'openai_oauth_responses_websockets_v2_mode'
                THEN 'ignored_by_openai_oauth_responses_websockets_v2_mode'
            WHEN legacy.key = 'responses_websockets_v2_enabled'
                 AND a.extra ? 'openai_oauth_responses_websockets_v2_enabled'
                THEN 'ignored_by_openai_oauth_responses_websockets_v2_enabled'
            WHEN legacy.key = 'openai_ws_enabled'
                 AND a.extra ? 'openai_oauth_responses_websockets_v2_mode'
                THEN 'ignored_by_openai_oauth_responses_websockets_v2_mode'
            WHEN legacy.key = 'openai_ws_enabled'
                 AND a.extra ? 'openai_oauth_responses_websockets_v2_enabled'
                THEN 'ignored_by_openai_oauth_responses_websockets_v2_enabled'
            WHEN legacy.key = 'openai_ws_enabled'
                 AND a.extra ? 'responses_websockets_v2_enabled'
                THEN 'ignored_by_responses_websockets_v2_enabled'
        END AS warning
    FROM accounts AS a
    CROSS JOIN (VALUES
        ('openai_oauth_passthrough'),
        ('openai_passthrough'),
        ('openai_oauth_responses_websockets_v2_mode'),
        ('openai_oauth_responses_websockets_v2_enabled'),
        ('responses_websockets_v2_enabled'),
        ('openai_ws_enabled'),
        ('openai_apikey_responses_websockets_v2_enabled'),
        ('openai_apikey_responses_websockets_v2_mode')
    ) AS legacy(key)
    WHERE a.platform = 'openai'
      AND a.type IN ('oauth', 'setup-token')
      AND a.deleted_at IS NULL
      AND a.extra ? legacy.key
)
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'openai_oauth_legacy_config_migration',
    'account:' || account_id::text || ':' || old_key,
    jsonb_strip_nulls(jsonb_build_object(
        'account_id', account_id,
        'account_type', account_type,
        'old_key', old_key,
        'old_value_class', old_value_class,
        'action', action,
        'new_key', new_key,
        'new_value', new_value,
        'warning', warning
    ))
FROM legacy_field_reports
ON CONFLICT (report_type, report_key) DO UPDATE
SET details = EXCLUDED.details;

WITH ws_targets AS (
    SELECT
        a.id,
        COALESCE(
            CASE
                WHEN a.extra ? 'openai_oauth_responses_websockets_v2_mode' THEN
                    CASE LOWER(BTRIM(a.extra ->> 'openai_oauth_responses_websockets_v2_mode'))
                        WHEN 'off' THEN 'off'
                        WHEN 'ctx_pool' THEN 'managed_session'
                        WHEN 'shared' THEN 'managed_session'
                        WHEN 'dedicated' THEN 'managed_session'
                        WHEN 'passthrough' THEN 'managed_session'
                        WHEN 'managed_session' THEN 'managed_session'
                    END
            END,
            CASE
                WHEN a.extra ? 'openai_oauth_responses_websockets_v2_enabled' THEN
                    CASE WHEN (a.extra ->> 'openai_oauth_responses_websockets_v2_enabled')::boolean THEN 'managed_session' ELSE 'off' END
                WHEN a.extra ? 'responses_websockets_v2_enabled' THEN
                    CASE WHEN (a.extra ->> 'responses_websockets_v2_enabled')::boolean THEN 'managed_session' ELSE 'off' END
                WHEN a.extra ? 'openai_ws_enabled' THEN
                    CASE WHEN (a.extra ->> 'openai_ws_enabled')::boolean THEN 'managed_session' ELSE 'off' END
            END
        ) AS target_mode
    FROM accounts AS a
    WHERE a.platform = 'openai'
      AND a.type IN ('oauth', 'setup-token')
      AND a.deleted_at IS NULL
), migrated_accounts AS (
    SELECT
        a.id,
        CASE
            WHEN ws_targets.target_mode IS NULL THEN
                a.extra
                - 'openai_oauth_passthrough'
                - 'openai_passthrough'
                - 'openai_oauth_responses_websockets_v2_mode'
                - 'openai_oauth_responses_websockets_v2_enabled'
                - 'responses_websockets_v2_enabled'
                - 'openai_ws_enabled'
                - 'openai_apikey_responses_websockets_v2_enabled'
                - 'openai_apikey_responses_websockets_v2_mode'
            ELSE
                jsonb_set(
                    a.extra
                    - 'openai_oauth_passthrough'
                    - 'openai_passthrough'
                    - 'openai_oauth_responses_websockets_v2_mode'
                    - 'openai_oauth_responses_websockets_v2_enabled'
                    - 'responses_websockets_v2_enabled'
                    - 'openai_ws_enabled'
                    - 'openai_apikey_responses_websockets_v2_enabled'
                    - 'openai_apikey_responses_websockets_v2_mode',
                    '{openai_oauth_ws_mode}',
                    to_jsonb(ws_targets.target_mode),
                    true
                )
        END AS next_extra
    FROM accounts AS a
    JOIN ws_targets ON ws_targets.id = a.id
    WHERE a.platform = 'openai'
      AND a.type IN ('oauth', 'setup-token')
      AND a.deleted_at IS NULL
      AND (
          a.extra ? 'openai_oauth_passthrough'
          OR a.extra ? 'openai_passthrough'
          OR a.extra ? 'openai_oauth_responses_websockets_v2_mode'
          OR a.extra ? 'openai_oauth_responses_websockets_v2_enabled'
          OR a.extra ? 'responses_websockets_v2_enabled'
          OR a.extra ? 'openai_ws_enabled'
          OR a.extra ? 'openai_apikey_responses_websockets_v2_enabled'
          OR a.extra ? 'openai_apikey_responses_websockets_v2_mode'
      )
)
UPDATE accounts AS a
SET extra = migrated_accounts.next_extra,
    updated_at = NOW()
FROM migrated_accounts
WHERE a.id = migrated_accounts.id
  AND a.extra <> migrated_accounts.next_extra;
