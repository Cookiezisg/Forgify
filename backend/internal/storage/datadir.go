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

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
