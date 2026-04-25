// Package state persists wallpaper-related state. Two files live in
// $XDG_STATE_HOME/wallforge/ (or ~/.local/state/wallforge/):
//
//   - last.json    — the wallpaper that was actually rendered most
//                    recently. Restored by `wallforge resume` and the
//                    watchdog when transitioning out of Paused mode.
//   - pending.json — a wallpaper the user *requested* while Paused.
//                    Apply on a paused system can't actually render,
//                    so we record the intent here. Resume picks
//                    pending if present (and consumes it), falling
//                    back to last otherwise.
//
// Splitting the two is what makes "apply X while paused, then plug in
// the charger" do the right thing without trampling the wallpaper
// that was active before the user went on battery.
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

// Path returns the on-disk location of the "last applied" record.
// XDG_STATE_HOME wins; otherwise the spec-default ~/.local/state/.
func Path() string {
	return resolvePath("last.json")
}

// PendingPath returns the location of the "pending intent" record —
// only present while a paused-mode Apply is queued for resume.
func PendingPath() string {
	return resolvePath("pending.json")
}

func resolvePath(file string) string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "wallforge", file)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "wallforge-"+file)
	}
	return filepath.Join(home, ".local", "state", "wallforge", file)
}

// Save writes entry to Path() (last.json). Intended to be called
// best-effort on successful apply; callers can ignore the error if
// the apply itself succeeded.
func Save(entry Entry) error {
	return writeJSON(Path(), entry)
}

// SavePending writes entry to PendingPath(). Used when Apply is
// requested but the system is in Paused mode — the rendering would
// be undone immediately, so we record the intent for the next
// resume to pick up.
func SavePending(entry Entry) error {
	return writeJSON(PendingPath(), entry)
}

// ClearPending removes the pending file. Resume calls this after
// consuming the entry. Missing file isn't an error — the file may
// not have existed in the first place.
func ClearPending() error {
	if err := os.Remove(PendingPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Load returns the last-applied entry, or a zero Entry + nil error
// when no file exists yet. Malformed files are an error — we'd
// rather fail loudly than silently reset state.
func Load() (Entry, error) {
	return readJSON(Path())
}

// LoadPending returns the pending entry (zero Entry + nil error if
// no pending file exists). Use ConsumePending when you've actually
// acted on it — leaving the file behind would re-fire next resume.
func LoadPending() (Entry, error) {
	return readJSON(PendingPath())
}

// ConsumePending loads pending and clears the file in one call.
// Returns the entry the caller should act on (zero Entry if no
// pending was queued).
func ConsumePending() (Entry, error) {
	e, err := LoadPending()
	if err != nil || e.Input == "" {
		return e, err
	}
	if err := ClearPending(); err != nil {
		// Don't lose the entry just because we failed to clear the
		// file — caller can still act, though the next resume might
		// repeat. Logged via the returned error.
		return e, fmt.Errorf("clear pending: %w", err)
	}
	return e, nil
}

func writeJSON(path string, entry Entry) error {
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

func readJSON(path string) (Entry, error) {
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
