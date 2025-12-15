package common

import (
	"os"
	"path/filepath"
)

// GetDataDir returns the base data directory path.
// Priority:
// 1. CNAP_DIR environment variable
// 2. $HOME/.cnap (default)
// 3. ./data (fallback if HOME is not set)
func GetDataDir() string {
	// 1. CNAP_DIR 환경변수 확인
	if cnapDir := os.Getenv("CNAP_DIR"); cnapDir != "" {
		return cnapDir
	}

	// 2. $HOME/.cnap 사용
	if homeDir := os.Getenv("HOME"); homeDir != "" {
		return filepath.Join(homeDir, ".cnap")
	}

	// 3. Fallback: ./data
	return "./data"
}

// GetWorkspaceDir returns the workspace directory path.
// Default: {DataDir}/workspace
func GetWorkspaceDir() string {
	if dir := os.Getenv("WORKSPACE_BASE_DIR"); dir != "" {
		return dir
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
	if sqlitePath := os.Getenv("SQLITE_DATABASE"); sqlitePath != "" {
		return sqlitePath
	}
	return filepath.Join(GetDataDir(), "cnap.db")
}
