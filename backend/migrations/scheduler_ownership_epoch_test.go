package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerOwnershipEpochMigrationIsDurableSingleton(t *testing.T) {
	raw, err := FS.ReadFile("163_scheduler_ownership_epoch.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))

	require.Contains(t, sql, "create table if not exists scheduler_ownership_epoch")
	require.Contains(t, sql, "singleton boolean primary key")
	require.Contains(t, sql, "epoch bigint not null")
	require.Contains(t, sql, "check (epoch > 0)")
	require.NotContains(t, sql, "sequence")
}
