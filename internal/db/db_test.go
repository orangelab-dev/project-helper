package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrate(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := Migrate(database); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='projects'").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "projects" {
		t.Fatalf("unexpected table %q", name)
	}
}

func TestSQLiteDSNWindowsDrivePath(t *testing.T) {
	dsn := sqliteDSN(`C:\Users\orange\project-helper\data\project-helper.db`)
	wantPrefix := "file:///C:/Users/orange/project-helper/data/project-helper.db?"
	if len(dsn) < len(wantPrefix) || dsn[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("dsn = %q, want prefix %q", dsn, wantPrefix)
	}
	for _, part := range []string{
		"_pragma=foreign_keys%281%29",
		"_pragma=journal_mode%28WAL%29",
		"_pragma=busy_timeout%285000%29",
		"_txlock=immediate",
	} {
		if !strings.Contains(dsn, part) {
			t.Fatalf("dsn = %q, missing %q", dsn, part)
		}
	}
}
