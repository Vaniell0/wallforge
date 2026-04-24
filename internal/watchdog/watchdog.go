// Package watchdog suspends wallpaper backends while the machine is on
// battery. Only video/scene backends meaningfully benefit — a static
// image has no runtime cost — so the watchdog calls engine.StopAll on
// a battery transition and triggers apply.ByInput(last) on AC return.
//
// We poll /sys/class/power_supply/BAT*/status rather than subscribe to
// UPower / power-profiles-daemon DBus so wallforge keeps its stdlib-only
// dependency posture.
package watchdog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PowerState is binary on purpose — "battery" vs "AC". Finer-grained
// detection (battery level, low-power profile) can layer on top later.
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

// Watchdog polls Detect on Interval and dispatches transitions to
// OnBattery / OnAC. State is tracked so each function fires exactly
// once per transition.
type Watchdog struct {
	Interval  time.Duration
	OnBattery func()
	OnAC      func()
	detect    func() PowerState // overridable in tests
}

// New constructs a Watchdog with the real Detect. Callers can replace
// the detect field directly in tests if they need to inject state.
func New(interval time.Duration, onBattery, onAC func()) *Watchdog {
	return &Watchdog{
		Interval:  interval,
		OnBattery: onBattery,
		OnAC:      onAC,
		detect:    Detect,
	}
}

// Run blocks until ctx is cancelled. The first tick snapshots the
// initial state and fires the corresponding callback — a fresh boot
// on battery still lets the user see the "battery" path run.
func (w *Watchdog) Run(ctx context.Context) error {
	if w.detect == nil {
		w.detect = Detect
	}
	// Initial fire so callers don't have to reconcile state themselves.
	state := w.detect()
	w.dispatch(state)
	last := state

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			now := w.detect()
			if now != last {
				w.dispatch(now)
				last = now
			}
		}
	}
}

func (w *Watchdog) dispatch(s PowerState) {
	switch s {
	case StateBattery:
		if w.OnBattery != nil {
			w.OnBattery()
		}
	case StateAC:
		if w.OnAC != nil {
			w.OnAC()
		}
	}
}
