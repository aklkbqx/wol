package store

import "testing"

func TestOpenWithConfigRejectsNonSQLite(t *testing.T) {
	if _, err := OpenWithConfig(Config{Driver: "redis"}); err == nil {
		t.Fatal("expected non-SQLite storage driver error")
	}
}

func TestOpenWithConfigDefaultsToSQLite(t *testing.T) {
	path := t.TempDir() + "/wol.db"
	repository, err := OpenWithConfig(Config{
		SQLitePath: path,
	})
	if err != nil {
		t.Fatalf("OpenWithConfig default: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("close SQLite repository: %v", err)
	}
}

func TestOpenWithConfigRejectsUnknownDriver(t *testing.T) {
	if _, err := OpenWithConfig(Config{Driver: "postgres", SQLitePath: "ignored.db"}); err == nil {
		t.Fatal("expected unsupported driver error")
	}
}
