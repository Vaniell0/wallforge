// Package watchdog suspends wallpaper backends when running them would
// burn battery the user wants to keep. Two signals feed the decision:
//
//   - sysfs `/sys/class/power_supply/BAT*/status` — AC vs battery
//   - power-profiles-daemon `powerprofilesctl get` — performance / balanced / power-saver
//
// The effective state is a single boolean — should we be paused? — that
// fires OnPause / OnResume callbacks on transitions. Only video/scene
// backends meaningfully benefit from pausing; static images have zero
// runtime cost, but engine.StopAll handles them uniformly.
//
// We poll instead of subscribing to UPower / ppd D-Bus signals so
// wallforge keeps its stdlib-only dependency posture.
package watchdog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Vaniell0/wallforge/internal/power"
)

// PowerState is binary on purpose — "battery" vs "AC". Finer-grained
// distinctions live in power.Profile, not here.
type PowerState int

const (
	StateUnknown PowerState = iota
	StateAC
	StateBattery
)

func (s PowerState) String() string {
	switch s {
	case StateAC:
		return "ac"
	case StateBattery:
		return "battery"
	}
	return "unknown"
}

// Detect reports the current power state by scanning every BAT* node
// under /sys/class/power_supply. A battery with status "Discharging"
// means we're on battery; any other status (Charging, Full, Not
// charging) means AC is plugged in. Desktop machines with no battery
// at all return StateAC.
func Detect() PowerState {
	return detectIn("/sys/class/power_supply")
}

func detectIn(root string) PowerState {
	matches, err := filepath.Glob(filepath.Join(root, "BAT*/status"))
	if err != nil || len(matches) == 0 {
		return StateAC
	}
	// If any battery reports Discharging, treat the whole system as
	// on battery. Users rarely have two batteries charging at once.
	for _, p := range matches {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == "Discharging" {
			return StateBattery
		}
	}
	return StateAC
}

// Snapshot is the combined power view at a single instant. Exposed so
// the web-UI can render the same fields the watchdog acts on without
// duplicating the detection wiring.
type Snapshot struct {
	Power   PowerState
	Profile power.Profile
}

// Effective decides whether wallforge should be paused given the
// current snapshot and the user preference for power-saver behaviour.
// Returns the boolean and a short human-readable reason (empty when
// not paused) so callers can log / surface it in the UI.
func Effective(s Snapshot, pauseOnPowerSaver bool) (paused bool, reason string) {
	onBat := s.Power == StateBattery
	onSaver := pauseOnPowerSaver && s.Profile == power.ProfilePowerSaver
	switch {
	case onBat && onSaver:
		return true, "battery+power-saver"
	case onBat:
		return true, "battery"
	case onSaver:
		return true, "power-saver"
	}
	return false, ""
}

// Watchdog polls the combined power signal on Interval and dispatches
// pause/resume transitions exactly once per change. Detection seams
// (detectPower / detectProfile) are overridable for tests.
type Watchdog struct {
	Interval          time.Duration
	PauseOnPowerSaver bool
	OnPause           func(reason string)
	OnResume          func()

	detectPower   func() PowerState
	detectProfile func() power.Profile
}

// New constructs a Watchdog wired to the real sysfs + ppd detectors.
// Callers can replace detectPower / detectProfile directly for tests.
func New(interval time.Duration, pauseOnPowerSaver bool, onPause func(reason string), onResume func()) *Watchdog {
	return &Watchdog{
		Interval:          interval,
		PauseOnPowerSaver: pauseOnPowerSaver,
		OnPause:           onPause,
		OnResume:          onResume,
		detectPower:       Detect,
		detectProfile:     defaultProfileDetector,
	}
}

// defaultProfileDetector wraps power.Detect and silently maps "ppd not
// installed" to ProfileUnknown — the watchdog should keep working on
// hosts without ppd. Real errors (ran-but-failed) bubble up via the
// log line below; we still return Unknown so the effective decision
// is "no opinion" rather than "force pause".
func defaultProfileDetector() power.Profile {
	p, err := power.Detect()
	if err != nil && !errors.Is(err, power.ErrNotInstalled) {
		fmt.Fprintf(os.Stderr, "wallforge watchdog: ppd probe failed: %v\n", err)
	}
	return p
}

// Snapshot returns the current power view without firing callbacks.
// Web-UI and tests use this; the Run loop computes its own snapshot
// each tick.
func (w *Watchdog) Snapshot() Snapshot {
	if w.detectPower == nil {
		w.detectPower = Detect
	}
	if w.detectProfile == nil {
		w.detectProfile = defaultProfileDetector
	}
	return Snapshot{Power: w.detectPower(), Profile: w.detectProfile()}
}

// Run blocks until ctx is cancelled. The first tick snapshots the
// initial state and fires the corresponding callback — a fresh boot
// already on battery (or in power-saver) still lets the user see the
// pause path execute.
func (w *Watchdog) Run(ctx context.Context) error {
	if w.detectPower == nil {
		w.detectPower = Detect
	}
	if w.detectProfile == nil {
		w.detectProfile = defaultProfileDetector
	}
	paused, reason := Effective(w.Snapshot(), w.PauseOnPowerSaver)
	w.dispatch(paused, reason)
	last := paused
	lastReason := reason

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			now, r := Effective(w.Snapshot(), w.PauseOnPowerSaver)
			// Fire on either a paused-flag flip or, while still paused,
			// a change in reason (battery → battery+power-saver). The
			// latter helps logs make sense when the user toggles ppd
			// while the laptop is unplugged.
			if now != last || (now && r != lastReason) {
				w.dispatch(now, r)
				last = now
				lastReason = r
			}
		}
	}
}

func (w *Watchdog) dispatch(paused bool, reason string) {
	if paused {
		if w.OnPause != nil {
			w.OnPause(reason)
		}
		return
	}
	if w.OnResume != nil {
		w.OnResume()
	}
}
