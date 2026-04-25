package watchdog

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Vaniell0/wallforge/internal/power"
)

func TestDetectIn(t *testing.T) {
	tests := []struct {
		name      string
		layout    map[string]string
		wantState PowerState
	}{
		{"no battery dir", map[string]string{}, StateAC},
		{"single battery discharging", map[string]string{"BAT0/status": "Discharging\n"}, StateBattery},
		{"single battery charging", map[string]string{"BAT0/status": "Charging\n"}, StateAC},
		{"two batteries, one discharging", map[string]string{
			"BAT0/status": "Full\n",
			"BAT1/status": "Discharging\n",
		}, StateBattery},
		{"two batteries, both full", map[string]string{
			"BAT0/status": "Full\n",
			"BAT1/status": "Full\n",
		}, StateAC},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tc.layout {
				full := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := detectIn(root); got != tc.wantState {
				t.Errorf("detectIn = %v, want %v", got, tc.wantState)
			}
		})
	}
}

func TestParsePolicy(t *testing.T) {
	cases := map[string]PowerSaverPolicy{
		"":         PolicyReduce,
		"reduce":   PolicyReduce,
		"REDUCE":   PolicyReduce,
		"  pause ": PolicyPause,
		"ignore":   PolicyIgnore,
		"garbage":  PolicyReduce, // unknown → safe default
	}
	for in, want := range cases {
		if got := ParsePolicy(in); got != want {
			t.Errorf("ParsePolicy(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEffectiveMode(t *testing.T) {
	cases := []struct {
		name       string
		snap       Snapshot
		policy     PowerSaverPolicy
		wantMode   Mode
		wantReason string
	}{
		{
			name:     "ac + performance → normal",
			snap:     Snapshot{Power: StateAC, Profile: power.ProfilePerformance},
			policy:   PolicyReduce,
			wantMode: ModeNormal,
		},
		{
			name:     "ac + balanced → normal",
			snap:     Snapshot{Power: StateAC, Profile: power.ProfileBalanced},
			policy:   PolicyReduce,
			wantMode: ModeNormal,
		},
		{
			name:       "ac + power-saver, reduce → low-power",
			snap:       Snapshot{Power: StateAC, Profile: power.ProfilePowerSaver},
			policy:     PolicyReduce,
			wantMode:   ModeLowPower,
			wantReason: "power-saver",
		},
		{
			name:       "ac + power-saver, pause → paused",
			snap:       Snapshot{Power: StateAC, Profile: power.ProfilePowerSaver},
			policy:     PolicyPause,
			wantMode:   ModePaused,
			wantReason: "power-saver",
		},
		{
			name:     "ac + power-saver, ignore → normal",
			snap:     Snapshot{Power: StateAC, Profile: power.ProfilePowerSaver},
			policy:   PolicyIgnore,
			wantMode: ModeNormal,
		},
		{
			name:       "battery alone → paused (battery)",
			snap:       Snapshot{Power: StateBattery, Profile: power.ProfileBalanced},
			policy:     PolicyReduce,
			wantMode:   ModePaused,
			wantReason: "battery",
		},
		{
			name:       "battery + power-saver → paused (battery wins)",
			snap:       Snapshot{Power: StateBattery, Profile: power.ProfilePowerSaver},
			policy:     PolicyReduce,
			wantMode:   ModePaused,
			wantReason: "battery",
		},
		{
			name:       "battery + power-saver, pause policy → still battery",
			snap:       Snapshot{Power: StateBattery, Profile: power.ProfilePowerSaver},
			policy:     PolicyPause,
			wantMode:   ModePaused,
			wantReason: "battery",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMode, gotReason := EffectiveMode(c.snap, c.policy)
			if gotMode != c.wantMode || gotReason != c.wantReason {
				t.Errorf("EffectiveMode = (%v, %q), want (%v, %q)",
					gotMode, gotReason, c.wantMode, c.wantReason)
			}
		})
	}
}

type tick struct {
	s PowerState
	p power.Profile
}

// driver wires a deterministic timeline through the watchdog detector
// seams. Each call to detectProfile (the second of the pair) advances
// the index, mirroring how detectPower would be called first per Run
// iteration.
type driver struct {
	mu       sync.Mutex
	timeline []tick
	idx      int
}

func (d *driver) power() PowerState {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx >= len(d.timeline) {
		return d.timeline[len(d.timeline)-1].s
	}
	return d.timeline[d.idx].s
}

func (d *driver) profile() power.Profile {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx >= len(d.timeline) {
		return d.timeline[len(d.timeline)-1].p
	}
	p := d.timeline[d.idx].p
	d.idx++
	return p
}

func TestWatchdog_FiresOnTransitions(t *testing.T) {
	d := &driver{timeline: []tick{
		{StateBattery, power.ProfileBalanced},   // → paused (battery)
		{StateBattery, power.ProfileBalanced},   // unchanged
		{StateAC, power.ProfileBalanced},        // → normal
		{StateAC, power.ProfilePowerSaver},      // → low-power
		{StateAC, power.ProfileBalanced},        // → normal
	}}

	var (
		mu          sync.Mutex
		transitions []Mode
		reasons     []string
	)
	w := &Watchdog{
		Interval: 5 * time.Millisecond,
		Policy:   PolicyReduce,
		OnModeChange: func(mode Mode, reason string) {
			mu.Lock()
			transitions = append(transitions, mode)
			reasons = append(reasons, reason)
			mu.Unlock()
		},
	}
	w.detectPower = d.power
	w.detectProfile = d.profile

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)

	mu.Lock()
	defer mu.Unlock()

	// Expected ordered modes (initial fire counts): paused, normal, low-power, normal.
	want := []Mode{ModePaused, ModeNormal, ModeLowPower, ModeNormal}
	if len(transitions) < len(want) {
		t.Fatalf("got %d transitions, want at least %d (%v)", len(transitions), len(want), transitions)
	}
	for i, m := range want {
		if transitions[i] != m {
			t.Errorf("transitions[%d] = %v, want %v (full: %v)", i, transitions[i], m, transitions)
		}
	}
	// Low-power transition must carry the "power-saver" reason.
	if reasons[2] != "power-saver" {
		t.Errorf("reasons[2] = %q, want power-saver (full: %v)", reasons[2], reasons)
	}
}

func TestWatchdog_NoCallbackNoCrash(t *testing.T) {
	w := &Watchdog{Interval: 5 * time.Millisecond, Policy: PolicyReduce}
	w.detectPower = func() PowerState { return StateAC }
	w.detectProfile = func() power.Profile { return power.ProfileBalanced }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)
}

func TestWatchdog_PausePolicyOnPowerSaver(t *testing.T) {
	// Verify policy=pause downgrades a power-saver profile from
	// LowPower (default) to Paused.
	d := &driver{timeline: []tick{
		{StateAC, power.ProfilePerformance},
		{StateAC, power.ProfilePowerSaver},
	}}
	var (
		mu     sync.Mutex
		modes  []Mode
	)
	w := &Watchdog{
		Interval: 5 * time.Millisecond,
		Policy:   PolicyPause,
		OnModeChange: func(m Mode, _ string) {
			mu.Lock()
			modes = append(modes, m)
			mu.Unlock()
		},
	}
	w.detectPower = d.power
	w.detectProfile = d.profile

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(modes) < 2 || modes[0] != ModeNormal || modes[1] != ModePaused {
		t.Errorf("modes = %v, want first two = [normal, paused]", modes)
	}
}
