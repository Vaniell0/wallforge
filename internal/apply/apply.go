// Package apply resolves a user-facing wallpaper input (filesystem path
// or Steam Workshop ID) and dispatches it to the right backend. Both the
// CLI (`wallforge apply`, `wallforge shuffle`) and the web-UI go through
// this entry point so classification stays in one place.
package apply

import (
	"fmt"
	"time"

	"github.com/Vaniell0/wallforge/internal/config"
	"github.com/Vaniell0/wallforge/internal/engine"
	"github.com/Vaniell0/wallforge/internal/state"
	"github.com/Vaniell0/wallforge/internal/steam"
	"github.com/Vaniell0/wallforge/internal/watchdog"
)

// Result describes the applied wallpaper for caller-side logging.
type Result struct {
	Kind    string // image, video, scene
	Backend string // swww, mpvpaper, linux-wallpaperengine
	Title   string // from project.json, empty for bare files
	Path    string // path handed to the backend
}

// Overridable seams so tests can exercise ByInput without touching the
// real Steam tree or executing backend processes. Production code uses
// the real implementations by default.
var (
	resolveSteam  = steam.Resolve
	selectBackend = engine.Select
	saveState     = state.Save
	stopOthers    = engine.StopOthers
	detectMode    = defaultDetectMode
)

// defaultDetectMode polls the same signals the watchdog Run loop uses,
// without requiring a long-lived Watchdog instance. Cheap on demand
// (one sysfs read + one short subprocess).
func defaultDetectMode(cfg config.Config) watchdog.Mode {
	w := watchdog.New(0, watchdog.ParsePolicy(cfg.Watchdog.PowerSaverPolicy), nil)
	mode, _ := watchdog.EffectiveMode(w.Snapshot(), w.Policy)
	return mode
}

// ByInput classifies input, runs the backend and returns a Result on
// success. Numeric inputs are resolved against the Steam Workshop content
// directory; everything else is a filesystem path.
//
// The current power mode is auto-detected and the backend is constructed
// from a mode-adjusted Config — LowPower swaps in battery_mpv_opts and
// fps_battery; Paused is honoured by stopping every backend instead of
// applying. Watchdog re-applies on mode transitions, so a profile flip
// after Apply still produces the right rendering.
func ByInput(cfg config.Config, input string) (Result, error) {
	mode := detectMode(cfg)
	if mode == watchdog.ModePaused {
		// Apply on a paused system would just be undone by the
		// watchdog on its next tick. Persist the request so a later
		// resume can pick it up, but don't render anything now.
		_ = saveState(state.Entry{Input: input, AppliedAt: time.Now().UTC()})
		return Result{}, fmt.Errorf("paused (battery / power-saver) — wallpaper recorded for resume but not rendered")
	}
	effective := cfg
	if mode == watchdog.ModeLowPower {
		effective = cfg.ForLowPower()
	}

	path := input
	if IsNumericID(input) {
		resolved, err := resolveSteam(cfg.Steam.Root, input)
		if err != nil {
			return Result{}, err
		}
		path = resolved
	}
	target, err := engine.Detect(path)
	if err != nil {
		return Result{}, err
	}
	backend, err := selectBackend(target, effective)
	if err != nil {
		return Result{}, err
	}
	// Clear any layer-surface the *other* backends are holding before
	// we paint. Otherwise a stale mpvpaper / lwpe window keeps rendering
	// over the new swww image and the user sees "nothing happened".
	stopOthers(backend, effective)
	if err := backend.Apply(target.Path); err != nil {
		return Result{}, fmt.Errorf("%s: %w", backend.Name(), err)
	}
	// Best-effort state persist — a failed write must not fail an
	// otherwise successful apply. The user cares about their wallpaper
	// being set; resume-after-reboot is a nice-to-have on top.
	_ = saveState(state.Entry{Input: input, AppliedAt: time.Now().UTC()})
	r := Result{
		Kind:    target.Kind.String(),
		Backend: backend.Name(),
		Path:    target.Path,
	}
	if target.Project != nil {
		r.Title = target.Project.Title
	}
	return r, nil
}

// IsNumericID reports whether s is a non-empty decimal string — the
// heuristic used to treat an argument as a Steam Workshop ID.
func IsNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
