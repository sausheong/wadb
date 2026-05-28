package macimport

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sausheong/wadb/internal/db"
)

// setupFixture builds a fresh ChatStorage.sqlite at <tempdir>/src.sqlite
// from the .sql files in testdata/, and a fresh wadb.db at <tempdir>/dst.sqlite
// with all migrations applied. Returns (srcPath, dstQueries).
func setupFixture(t *testing.T) (string, *db.Queries) {
	t.Helper()
	tmp := t.TempDir()

	srcPath := filepath.Join(tmp, "src.sqlite")
	src, err := sql.Open("sqlite", "file:"+srcPath)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}

	for _, file := range []string{"testdata/fixture_schema.sql", "testdata/fixture.sql"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := src.ExecContext(context.Background(), string(raw)); err != nil {
			t.Fatalf("exec %s: %v", file, err)
		}
	}
	if err := src.Close(); err != nil {
		t.Fatalf("close src: %v", err)
	}

	dstConn, err := db.Open(filepath.Join(tmp, "dst.sqlite"))
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	if err := db.Migrate(context.Background(), dstConn); err != nil {
		t.Fatalf("migrate dst: %v", err)
	}
	t.Cleanup(func() { dstConn.Close() })

	return srcPath, db.NewQueries(dstConn)
}

func TestImport_SkeletonRuns(t *testing.T) {
	srcPath, q := setupFixture(t)
	imp, err := New(srcPath, q)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer imp.Close()
	stats, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Stubs return zero. This test exists to verify the wiring compiles
	// and the pipeline runs end-to-end before per-step tests in Task 5.
	if stats.Errors != 0 {
		t.Errorf("unexpected errors: %d", stats.Errors)
	}
}
