package workshop

import (
	"os"
	"path/filepath"
)

// DefaultCacheDir returns the XDG-compliant directory where downloaded
// workshop items live. Respects $XDG_DATA_HOME, falls back to
// ~/.local/share/wallforge/workshop.
func DefaultCacheDir() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "wallforge", "workshop")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Last-resort: CWD — caller will surface the error when it tries
		// to write. Still typed as a cache dir so the rest of the code
		// stays uniform.
		return filepath.Join(".", ".wallforge-cache")
	}
	return filepath.Join(home, ".local", "share", "wallforge", "workshop")
}

// ItemDir returns the cache directory for a specific workshop item.
func ItemDir(cacheDir, workshopID string) string {
	return filepath.Join(cacheDir, workshopID)
}
