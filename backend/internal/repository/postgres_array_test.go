package repository

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestPostgresInt64ArrayEncodesForPgx(t *testing.T) {
	tests := []struct {
		name string
		ids  []int64
		want string
	}{
		{name: "nil slice becomes empty postgres array", ids: nil, want: "{}"},
		{name: "empty slice", ids: []int64{}, want: "{}"},
		{name: "values", ids: []int64{1, 2, -3}, want: "{1,2,-3}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := pgtype.NewMap().Encode(pgtype.Int8ArrayOID, pgtype.TextFormatCode, postgresInt64Array(tt.ids), nil)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(encoded))
		})
	}
}

func TestScanPostgresInt64Array(t *testing.T) {
	var got []int64

	err := scanPostgresInt64Array(&got).Scan([]byte(`{1,2,-3}`))
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, -3}, got)
}

func TestScanPostgresInt64ArrayEmptyArray(t *testing.T) {
	got := []int64{99}

	err := scanPostgresInt64Array(&got).Scan(`{}`)
	require.NoError(t, err)
	require.Empty(t, got)
}
