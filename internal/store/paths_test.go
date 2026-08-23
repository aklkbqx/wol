package store

import (
	"path/filepath"
	"testing"
)

func TestDefaultDataDirAndDatabasePath(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Test WOL_DATA_DIR override
	t.Setenv("WOL_DATA_DIR", tempDir)
	t.Setenv("WOL_DB", "")
	if got := DefaultDataDir(); got != tempDir {
		t.Fatalf("DefaultDataDir() = %q, want %q", got, tempDir)
	}
	expectedDB := filepath.Join(tempDir, "wol.db")
	if got := DefaultDatabasePath(); got != expectedDB {
		t.Fatalf("DefaultDatabasePath() = %q, want %q", got, expectedDB)
	}

	// 2. Test XDG_DATA_HOME override
	t.Setenv("WOL_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", tempDir)
	expectedXDG := filepath.Join(tempDir, "wol")
	if got := DefaultDataDir(); got != expectedXDG {
		t.Fatalf("DefaultDataDir() with XDG_DATA_HOME = %q, want %q", got, expectedXDG)
	}

	// 3. Test explicit WOL_DB override
	customDB := filepath.Join(tempDir, "custom.db")
	t.Setenv("WOL_DB", customDB)
	if got := DefaultDatabasePath(); got != customDB {
		t.Fatalf("DefaultDatabasePath() with WOL_DB = %q, want %q", got, customDB)
	}

	// 4. Test WOL_TARGET_DB override
	customTarget := filepath.Join(tempDir, "target.db")
	t.Setenv("WOL_TARGET_DB", customTarget)
	if got := DefaultMigrationTargetDB(); got != customTarget {
		t.Fatalf("DefaultMigrationTargetDB() with WOL_TARGET_DB = %q, want %q", got, customTarget)
	}
}

func TestDefaultWebDir(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("WOL_WEB_DIR", tempDir)
	if got := DefaultWebDir(); got != tempDir {
		t.Fatalf("DefaultWebDir() = %q, want %q", got, tempDir)
	}
}
