// Package state persists the most recent apply target so wallforge can
// restore the previous wallpaper after a session restart. State lives in
// $XDG_STATE_HOME/wallforge/last.json (or ~/.local/state/wallforge/...
// when the env var is unset).
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Entry is the single stored record. Kept tiny on purpose — we may
// store more in the future (per-workspace bindings, etc.) but those
// belong in their own files, not bundled here.
type Entry struct {
	Input     string    `json:"input"`
	AppliedAt time.Time `json:"applied_at"`
}

// Path returns the on-disk location. XDG_STATE_HOME wins; otherwise
// the spec-default ~/.local/state/ is used.
func Path() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "wallforge", "last.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "wallforge-state.json")
	}
	return filepath.Join(home, ".local", "state", "wallforge", "last.json")
}

// Save writes entry to Path(), creating the directory as needed.
// Intended to be called best-effort on successful apply; callers can
// ignore the error if the apply itself succeeded.
func Save(entry Entry) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	// Atomic rename so a crashed process never leaves a half-written
	// file that trips Load next boot.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return os.Rename(tmp, path)
}

// Load returns the stored entry, or a zero Entry + nil error when no
// state file exists yet. Malformed files are an error — we'd rather
// fail loudly than silently reset the user's last wallpaper.
func Load() (Entry, error) {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Entry{}, nil
		}
		return Entry{}, fmt.Errorf("read %s: %w", path, err)
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return Entry{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return e, nil
}
