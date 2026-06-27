package repository

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestIsUniqueConstraintViolationTypedNilPgError(t *testing.T) {
	var pgErr *pgconn.PgError
	var err error = pgErr

	require.False(t, isUniqueConstraintViolation(err))
}
