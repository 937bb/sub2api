-- Add BTREE expression index on credentials->>'plan_type' to support
-- admin account list filtering by plan_type without sequential scan.
--
-- Uses expression index rather than GIN: plan_type is the only JSON path
-- queried on credentials, and expression indexes are smaller/faster than
-- GIN for exact equality comparisons via ->>.
--
-- _notx suffix: this migration creates an index concurrently and is
-- excluded from transaction-wrapped migration runs (see migrations/README.md).

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_credentials_plan_type
    ON accounts ((credentials->>'plan_type'));
