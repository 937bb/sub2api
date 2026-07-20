ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS time_billing_rules JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE groups
    DROP CONSTRAINT IF EXISTS groups_time_billing_rules_array;

ALTER TABLE groups
    ADD CONSTRAINT groups_time_billing_rules_array CHECK (
        jsonb_typeof(time_billing_rules) = 'array'
    );

UPDATE groups
SET time_billing_rules = jsonb_build_array(jsonb_build_object(
        'id', 'legacy-' || id::text,
        'enabled', TRUE,
        'start', peak_start,
        'end', peak_end,
        'rate_multiplier', peak_rate_multiplier
    ))
WHERE jsonb_array_length(time_billing_rules) = 0
  AND peak_rate_enabled = TRUE
  AND peak_start <> ''
  AND peak_end <> '';
