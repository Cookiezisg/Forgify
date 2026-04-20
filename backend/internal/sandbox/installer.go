package sandbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/sunweilin/forgify/internal/storage"
)

// uvBinName returns the platform-specific uv binary name.
func uvBinName() string {
	if runtime.GOOS == "windows" {
		return "uv.exe"
	}
	return "uv"
}

// findUV locates the uv binary. Checks:
// 1. Same directory as the Forgify executable (bundled)
// 2. Common install locations (~/.local/bin, ~/.cargo/bin)
// 3. System PATH
func findUV() (string, error) {
	name := uvBinName()

	// Check bundled location
	exe, err := os.Executable()
	if err == nil {
		bundled := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(bundled); err == nil {
			return bundled, nil
		}
	}

	// Check common install locations (uv installs to ~/.local/bin)
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, dir := range []string{".local/bin", ".cargo/bin"} {
			p := filepath.Join(home, dir, name)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}

	// Fall back to system PATH
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("uv not found: install uv (curl -LsSf https://astral.sh/uv/install.sh | sh)")
	}
	return path, nil
}

// venvKey creates a deterministic hash for a set of requirements.
func venvKey(requirements []string) string {
	if len(requirements) == 0 {
		return "base"
	}
	sorted := make([]string, len(requirements))
	copy(sorted, requirements)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return fmt.Sprintf("%x", h[:8])
}

// VenvsDir returns the root directory for virtual environments.
func VenvsDir() string {
	return filepath.Join(storage.DataDir(), "venvs")
}

// EnsureVenv ensures a virtual environment with the given requirements exists.
// Returns the path to the venv directory. Uses a sentinel file for cache checking.
func EnsureVenv(ctx context.Context, requirements []string) (string, error) {
	uv, err := findUV()
	if err != nil {
		return "", err
	}

	key := venvKey(requirements)
	venvDir := filepath.Join(VenvsDir(), key)

	// Cache hit: sentinel file exists
	sentinel := filepath.Join(venvDir, ".installed")
	if _, err := os.Stat(sentinel); err == nil {
		return venvDir, nil
	}

	// Create venv (uv auto-downloads Python 3.12 if needed)
	if err := os.MkdirAll(filepath.Dir(venvDir), 0755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, uv, "venv", "--python", "3.12", venvDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create venv failed: %w\n%s", err, out)
	}

	// Install requirements
	if len(requirements) > 0 {
		args := append([]string{"pip", "install", "--quiet"}, requirements...)
		cmd = exec.CommandContext(ctx, uv, args...)
		cmd.Env = append(os.Environ(), "VIRTUAL_ENV="+venvDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			// Clean up failed venv
			os.RemoveAll(venvDir)
			return "", fmt.Errorf("install dependencies failed (%s): %w\n%s",
				strings.Join(requirements, ", "), err, out)
		}
	}

	// Write sentinel
	os.WriteFile(sentinel, []byte("ok"), 0644)
	return venvDir, nil
}

// PythonBin returns the python binary path inside a venv.
func PythonBin(venvDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", "python.exe")
	}
	return filepath.Join(venvDir, "bin", "python")
}
