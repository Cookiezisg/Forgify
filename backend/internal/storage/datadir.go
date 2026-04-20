package storage

import (
	"os"
	"path/filepath"
	"runtime"
)

func DefaultDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Forgify")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Forgify")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "Forgify")
	}
}

// DataDir returns the active data directory (set during Init).
// Falls back to DefaultDataDir if Init hasn't been called.
var dataDir string

func DataDir() string {
	if dataDir != "" {
		return dataDir
	}
	return DefaultDataDir()
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
