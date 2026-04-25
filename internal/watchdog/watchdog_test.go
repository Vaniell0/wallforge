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
		layout    map[string]string // rel path → content
		wantState PowerState
	}{
		{
			name:      "no battery dir",
			layout:    map[string]string{},
			wantState: StateAC,
		},
		{
			name:      "single battery discharging",
			layout:    map[string]string{"BAT0/status": "Discharging\n"},
			wantState: StateBattery,
		},
		{
			name:      "single battery charging",
			layout:    map[string]string{"BAT0/status": "Charging\n"},
			wantState: StateAC,
		},
		{
			name: "two batteries, one discharging",
			layout: map[string]string{
				"BAT0/status": "Full\n",
				"BAT1/status": "Discharging\n",
			},
			wantState: StateBattery,
		},
		{
			name: "two batteries, both full",
			layout: map[string]string{
				"BAT0/status": "Full\n",
				"BAT1/status": "Full\n",
			},
			wantState: StateAC,
		},
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

func TestEffective(t *testing.T) {
	cases := []struct {
		name              string
		s                 Snapshot
		pauseOnPowerSaver bool
		wantPaused        bool
		wantReason        string
	}{
		{
			name: "ac + performance → resume",
			s:    Snapshot{Power: StateAC, Profile: power.ProfilePerformance},
		},
		{
			name:       "battery alone → pause battery",
			s:          Snapshot{Power: StateBattery, Profile: power.ProfileBalanced},
			wantPaused: true, wantReason: "battery",
		},
		{
			name:              "ac + power-saver, opt-in → pause",
			s:                 Snapshot{Power: StateAC, Profile: power.ProfilePowerSaver},
			pauseOnPowerSaver: true,
			wantPaused:        true, wantReason: "power-saver",
		},
		{
			name:              "ac + power-saver, opt-out → resume",
			s:                 Snapshot{Power: StateAC, Profile: power.ProfilePowerSaver},
			pauseOnPowerSaver: false,
		},
		{
			name:              "battery + power-saver → combined reason",
			s:                 Snapshot{Power: StateBattery, Profile: power.ProfilePowerSaver},
			pauseOnPowerSaver: true,
			wantPaused:        true, wantReason: "battery+power-saver",
		},
		{
			name: "ppd unknown on battery → still pauses for battery",
			s:    Snapshot{Power: StateBattery, Profile: power.ProfileUnknown},
			pauseOnPowerSaver: true,
			wantPaused: true, wantReason: "battery",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotPaused, gotReason := Effective(c.s, c.pauseOnPowerSaver)
			if gotPaused != c.wantPaused || gotReason != c.wantReason {
				t.Errorf("Effective = (%v, %q), want (%v, %q)",
					gotPaused, gotReason, c.wantPaused, c.wantReason)
			}
		})
	}
}

func TestWatchdog_FiresOnTransitions(t *testing.T) {
	type tick struct {
		s PowerState
		p power.Profile
	}
	var (
		mu          sync.Mutex
		pauseCount  int
		resumeCount int
		lastReason  string
		// Timeline: battery → AC → power-saver-on-AC → AC again.
		timeline = []tick{
			{StateBattery, power.ProfileBalanced},
			{StateBattery, power.ProfileBalanced},
			{StateAC, power.ProfileBalanced},
			{StateAC, power.ProfilePowerSaver},
			{StateAC, power.ProfileBalanced},
		}
		idx int
	)

	w := &Watchdog{
		Interval:          5 * time.Millisecond,
		PauseOnPowerSaver: true,
		OnPause: func(reason string) {
			mu.Lock()
			pauseCount++
			lastReason = reason
			mu.Unlock()
		},
		OnResume: func() {
			mu.Lock()
			resumeCount++
			mu.Unlock()
		},
	}
	w.detectPower = func() PowerState {
		mu.Lock()
		defer mu.Unlock()
		if idx >= len(timeline) {
			return timeline[len(timeline)-1].s
		}
		return timeline[idx].s
	}
	w.detectProfile = func() power.Profile {
		mu.Lock()
		defer mu.Unlock()
		if idx >= len(timeline) {
			return timeline[len(timeline)-1].p
		}
		p := timeline[idx].p
		idx++
		return p
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)

	mu.Lock()
	defer mu.Unlock()

	// Expected transitions:
	//   tick 0: battery        → pause (battery)
	//   tick 1: battery        → no fire (same state)
	//   tick 2: ac/balanced    → resume
	//   tick 3: ac/power-saver → pause (power-saver)
	//   tick 4: ac/balanced    → resume
	if pauseCount < 2 {
		t.Errorf("OnPause called %d times, want ≥ 2", pauseCount)
	}
	if resumeCount < 2 {
		t.Errorf("OnResume called %d times, want ≥ 2", resumeCount)
	}
	// Last pause should have been triggered by power-saver, not battery.
	if lastReason != "power-saver" {
		t.Errorf("lastReason = %q, want power-saver", lastReason)
	}
}

func TestWatchdog_NoCallbacksNoCrash(t *testing.T) {
	// Omitting both callbacks must still run cleanly.
	w := &Watchdog{Interval: 5 * time.Millisecond}
	w.detectPower = func() PowerState { return StateAC }
	w.detectProfile = func() power.Profile { return power.ProfileBalanced }

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)
}

func TestWatchdog_ReasonChangeWhilePaused(t *testing.T) {
	// While we stay paused, the reason can change (battery →
	// battery+power-saver). The dispatcher must re-fire OnPause so
	// the log line matches reality, even though the boolean didn't flip.
	var (
		mu      sync.Mutex
		reasons []string
	)
	timeline := []struct {
		s PowerState
		p power.Profile
	}{
		{StateBattery, power.ProfileBalanced},   // pause: battery
		{StateBattery, power.ProfilePowerSaver}, // re-fire: battery+power-saver
		{StateBattery, power.ProfilePowerSaver},
	}
	idx := 0
	w := &Watchdog{
		Interval:          5 * time.Millisecond,
		PauseOnPowerSaver: true,
		OnPause: func(reason string) {
			mu.Lock()
			reasons = append(reasons, reason)
			mu.Unlock()
		},
	}
	w.detectPower = func() PowerState {
		mu.Lock()
		defer mu.Unlock()
		if idx >= len(timeline) {
			return timeline[len(timeline)-1].s
		}
		return timeline[idx].s
	}
	w.detectProfile = func() power.Profile {
		mu.Lock()
		defer mu.Unlock()
		if idx >= len(timeline) {
			return timeline[len(timeline)-1].p
		}
		p := timeline[idx].p
		idx++
		return p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	// Want at least one "battery" reason then at least one
	// "battery+power-saver" reason.
	sawBattery := false
	sawBoth := false
	for _, r := range reasons {
		if r == "battery" {
			sawBattery = true
		}
		if r == "battery+power-saver" {
			sawBoth = true
		}
	}
	if !sawBattery || !sawBoth {
		t.Errorf("reasons = %v, want both 'battery' and 'battery+power-saver'", reasons)
	}
}
