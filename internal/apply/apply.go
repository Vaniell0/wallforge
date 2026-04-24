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
)

// ByInput classifies input, runs the backend and returns a Result on
// success. Numeric inputs are resolved against the Steam Workshop content
// directory; everything else is a filesystem path.
func ByInput(cfg config.Config, input string) (Result, error) {
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
	backend, err := selectBackend(target, cfg)
	if err != nil {
		return Result{}, err
	}
	// Clear any layer-surface the *other* backends are holding before
	// we paint. Otherwise a stale mpvpaper / lwpe window keeps rendering
	// over the new swww image and the user sees "nothing happened".
	stopOthers(backend, cfg)
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
