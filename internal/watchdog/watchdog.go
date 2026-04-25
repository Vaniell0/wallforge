// Package watchdog tracks the system power signal and fires callbacks
// on transitions between three operating modes:
//
//   - Normal     — full-quality rendering (AC + perf/balanced profile)
//   - LowPower   — reduced rendering (battery-tuned mpv opts, lower lwpe fps)
//   - Paused     — backends stopped (always on battery; opt-in on power-saver)
//
// Two signals feed the decision:
//
//   - sysfs `/sys/class/power_supply/BAT*/status` — AC vs battery
//   - power-profiles-daemon `powerprofilesctl get` — performance / balanced / power-saver
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

// Mode is the watchdog's effective output: what should wallforge be
// doing right now. Order matters for tests / logs but not for logic.
type Mode int

const (
	ModeUnknown Mode = iota
	ModeNormal
	ModeLowPower
	ModePaused
)

func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeLowPower:
		return "low-power"
	case ModePaused:
		return "paused"
	}
	return "unknown"
}

// PowerSaverPolicy decides how to react when ppd reports the
// power-saver profile. battery → ModePaused is hardcoded; this knob
// only affects the AC + power-saver corner.
type PowerSaverPolicy int

const (
	PolicyReduce PowerSaverPolicy = iota // default: drop to LowPower
	PolicyPause                          // full stop on power-saver
	PolicyIgnore                         // pretend ppd said balanced
)

// ParsePolicy maps a config string to the typed enum. Unknown values
// silently fall back to the default (Reduce) so a typo can't brick
// the watchdog.
func ParsePolicy(s string) PowerSaverPolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pause":
		return PolicyPause
	case "ignore":
		return PolicyIgnore
	}
	return PolicyReduce
}

func (p PowerSaverPolicy) String() string {
	switch p {
	case PolicyPause:
		return "pause"
	case PolicyIgnore:
		return "ignore"
	}
	return "reduce"
}

// Detect reports the current power state by scanning every BAT* node
// under /sys/class/power_supply. A battery with status "Discharging"
// means we're on battery; any other status means AC. Desktop machines
// with no battery at all return StateAC.
func Detect() PowerState {
	return detectIn("/sys/class/power_supply")
}

func detectIn(root string) PowerState {
	matches, err := filepath.Glob(filepath.Join(root, "BAT*/status"))
	if err != nil || len(matches) == 0 {
		return StateAC
	}
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
// the web-UI can render the same fields the watchdog acts on.
type Snapshot struct {
	Power   PowerState
	Profile power.Profile
}

// EffectiveMode collapses (Snapshot, Policy) into a single Mode + a
// short human-readable reason describing the dominant signal.
//
// Decision table:
//
//	battery        + (anything)        → Paused, "battery"
//	AC             + power-saver       → policy-driven
//	AC             + perf / balanced   → Normal
//
// Battery is non-negotiable: wallforge will not run video / scene
// backends on battery regardless of the ppd profile.
func EffectiveMode(s Snapshot, policy PowerSaverPolicy) (Mode, string) {
	if s.Power == StateBattery {
		return ModePaused, "battery"
	}
	if s.Profile == power.ProfilePowerSaver {
		switch policy {
		case PolicyPause:
			return ModePaused, "power-saver"
		case PolicyIgnore:
			return ModeNormal, ""
		}
		return ModeLowPower, "power-saver"
	}
	return ModeNormal, ""
}

// Watchdog polls the combined signal on Interval and dispatches
// OnModeChange exactly once per change. Detection seams (detectPower /
// detectProfile) are overridable for tests.
type Watchdog struct {
	Interval time.Duration
	Policy   PowerSaverPolicy
	// OnModeChange fires on every effective-mode transition (and on
	// the first tick so callers can reconcile state from boot).
	OnModeChange func(mode Mode, reason string)

	detectPower   func() PowerState
	detectProfile func() power.Profile
}

// New constructs a Watchdog wired to the real sysfs + ppd detectors.
func New(interval time.Duration, policy PowerSaverPolicy, onModeChange func(Mode, string)) *Watchdog {
	return &Watchdog{
		Interval:      interval,
		Policy:        policy,
		OnModeChange:  onModeChange,
		detectPower:   Detect,
		detectProfile: defaultProfileDetector,
	}
}

// defaultProfileDetector wraps power.Detect and silently maps "ppd not
// installed" to ProfileUnknown — the watchdog should keep working on
// hosts without ppd. Real errors (ran-but-failed) bubble to stderr;
// we still return Unknown so the effective decision is "no opinion".
func defaultProfileDetector() power.Profile {
	p, err := power.Detect()
	if err != nil && !errors.Is(err, power.ErrNotInstalled) {
		fmt.Fprintf(os.Stderr, "wallforge watchdog: ppd probe failed: %v\n", err)
	}
	return p
}

// Snapshot returns the current power view without firing callbacks.
// Web-UI and tests use this; the Run loop computes its own each tick.
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
// initial state and fires OnModeChange — a fresh boot already in
// LowPower / Paused still lets the caller see the corresponding path.
func (w *Watchdog) Run(ctx context.Context) error {
	if w.detectPower == nil {
		w.detectPower = Detect
	}
	if w.detectProfile == nil {
		w.detectProfile = defaultProfileDetector
	}
	mode, reason := EffectiveMode(w.Snapshot(), w.Policy)
	w.dispatch(mode, reason)
	lastMode := mode
	lastReason := reason

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			now, r := EffectiveMode(w.Snapshot(), w.Policy)
			// Re-fire on either a mode flip or, while staying in a
			// non-Normal mode, a change in reason — the dispatcher
			// log line stays coherent for the user that way.
			if now != lastMode || (now != ModeNormal && r != lastReason) {
				w.dispatch(now, r)
				lastMode = now
				lastReason = r
			}
		}
	}
}

func (w *Watchdog) dispatch(mode Mode, reason string) {
	if w.OnModeChange != nil {
		w.OnModeChange(mode, reason)
	}
}
