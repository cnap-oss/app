package common

import (
	"os"
	"path/filepath"
)

// GetDataDir returns the base data directory path.
// Priority:
// 1. CNAP_DIR from config
// 2. $HOME/.cnap (default)
// 3. ./data (fallback if HOME is not set)
func GetDataDir() string {
	cfg, err := LoadConfig()
	if err == nil && cfg.Directory.CNAPDir != "" {
		return cfg.Directory.CNAPDir
	}

	// Fallback: $HOME/.cnap 사용
	if homeDir := os.Getenv("HOME"); homeDir != "" {
		return filepath.Join(homeDir, ".cnap")
	}

	// Fallback: ./data
	return "./data"
}

// GetWorkspaceDir returns the workspace directory path.
// Default: {DataDir}/workspace
func GetWorkspaceDir() string {
	cfg, err := LoadConfig()
	if err == nil && cfg.Directory.WorkspaceBaseDir != "" {
		return cfg.Directory.WorkspaceBaseDir
	}
	return filepath.Join(GetDataDir(), "workspace")
}

// GetMessagesDir returns the messages directory path.
// Default: {DataDir}/messages
func GetMessagesDir() string {
	return filepath.Join(GetDataDir(), "messages")
}

// GetConfigsDir returns the configs directory path.
// Default: {DataDir}/configs
func GetConfigsDir() string {
	return filepath.Join(GetDataDir(), "configs")
}

// GetDatabasePath returns the SQLite database file path.
// Default: {DataDir}/cnap.db
func GetDatabasePath() string {
	cfg, err := LoadConfig()
	if err == nil && cfg.Directory.SQLiteDatabase != "" {
		return cfg.Directory.SQLiteDatabase
	}
	return filepath.Join(GetDataDir(), "cnap.db")
}
