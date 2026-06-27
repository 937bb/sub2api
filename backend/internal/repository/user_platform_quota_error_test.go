package repository

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestIsUserPlatformQuotaFKViolationTypedNilPgError(t *testing.T) {
	var pgErr *pgconn.PgError
	var err error = pgErr

	require.False(t, isUserPlatformQuotaFKViolation(err))
}
