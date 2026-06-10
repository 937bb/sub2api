package repository

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPostgresDriverMigrationContract(t *testing.T) {
	root := backendRoot(t)
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	if strings.Contains(string(goMod), "github.com/"+"lib/pq") {
		t.Fatalf("go.mod still depends on lib/pq; migrate the PostgreSQL driver to pgx/v5")
	}
	if !strings.Contains(string(goMod), "github.com/jackc/pgx/v5") {
		t.Fatalf("go.mod must require github.com/jackc/pgx/v5 for the PostgreSQL driver migration")
	}

	for _, violation := range scanPostgresDriverMigrationViolations(t, root) {
		t.Error(violation)
	}
}

func scanPostgresDriverMigrationViolations(t *testing.T, root string) []string {
	t.Helper()

	var violations []string
	legacyImport := "github.com/" + "lib/pq"
	legacyArray := "pq" + ".Array"
	legacyError := "pq" + ".Error"
	legacySQLOpen := "sql.Open(" + "\"postgres\""
	legacyEntOpen := "entsql.Open(" + "dialect.Postgres"

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".claude" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || filepath.Base(path) == "postgres_driver_migration_test.go" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(data)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		if strings.Contains(source, legacyImport) {
			violations = append(violations, rel+": imports lib/pq")
		}
		if strings.Contains(source, legacyArray) {
			violations = append(violations, rel+": uses pq.Array; pgx stdlib accepts native slices")
		}
		if strings.Contains(source, legacyError) {
			violations = append(violations, rel+": checks pq.Error; use pgconn.PgError")
		}
		if strings.Contains(source, legacySQLOpen) {
			violations = append(violations, rel+": opens legacy postgres driver; use pgx stdlib driver")
		}
		if strings.Contains(source, legacyEntOpen) {
			violations = append(violations, rel+": lets Ent open the legacy postgres driver; open pgx with database/sql and pass it to Ent")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan backend Go files: %v", err)
	}

	return violations
}

func backendRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
