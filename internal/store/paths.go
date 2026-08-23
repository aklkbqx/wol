package store

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultDataDir returns the standard directory used for persistent WOL data.
// It prioritizes WOL_DATA_DIR, then standard user data/config directories
// (e.g. ~/Library/Application Support/wol on macOS, ~/.config/wol or $XDG_DATA_HOME/wol on Linux),
// and falls back to ~/.wol in the user's home directory.
func DefaultDataDir() string {
	if dir := strings.TrimSpace(os.Getenv("WOL_DATA_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	if xdgData := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdgData != "" {
		return filepath.Join(filepath.Clean(xdgData), "wol")
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "wol")
	}
	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".wol")
	}
	return filepath.Join(".", ".wol")
}

// DefaultDatabasePath returns the default SQLite database path.
// It prioritizes WOL_DB, then <DefaultDataDir>/wol.db.
func DefaultDatabasePath() string {
	if db := strings.TrimSpace(os.Getenv("WOL_DB")); db != "" {
		return filepath.Clean(db)
	}
	return filepath.Join(DefaultDataDir(), "wol.db")
}

// DefaultMigrationTargetDB returns the default migration target SQLite path.
func DefaultMigrationTargetDB() string {
	if db := strings.TrimSpace(os.Getenv("WOL_TARGET_DB")); db != "" {
		return filepath.Clean(db)
	}
	return filepath.Join(DefaultDataDir(), "migration-target.db")
}

// DefaultWebDir returns the candidate directory for static web assets.
func DefaultWebDir() string {
	if dir := strings.TrimSpace(os.Getenv("WOL_WEB_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	if info, err := os.Stat("web/build"); err == nil && info.IsDir() {
		return "web/build"
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidate := filepath.Join(execDir, "web", "build")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		candidate = filepath.Join(execDir, "web-build")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	if dataDir := DefaultDataDir(); dataDir != "" {
		candidate := filepath.Join(dataDir, "web")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return "web/build"
}
