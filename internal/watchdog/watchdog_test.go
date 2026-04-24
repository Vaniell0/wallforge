package watchdog

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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

func TestWatchdog_FiresOnTransitions(t *testing.T) {
	var (
		mu       sync.Mutex
		batCount int
		acCount  int
		// Timeline of states the watchdog will observe on successive ticks.
		states = []PowerState{
			StateBattery, StateBattery, StateAC, StateAC, StateBattery, StateAC,
		}
		idx int
	)

	w := &Watchdog{
		Interval: 5 * time.Millisecond,
		OnBattery: func() {
			mu.Lock()
			batCount++
			mu.Unlock()
		},
		OnAC: func() {
			mu.Lock()
			acCount++
			mu.Unlock()
		},
	}
	w.detect = func() PowerState {
		mu.Lock()
		defer mu.Unlock()
		if idx >= len(states) {
			return states[len(states)-1]
		}
		s := states[idx]
		idx++
		return s
	}

	// Run for enough ticks to consume the whole timeline plus settling.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)

	mu.Lock()
	defer mu.Unlock()

	// Expected: initial=battery (fire OnBattery), then AC (fire OnAC),
	// then battery (fire OnBattery), then AC (fire OnAC) — 2 battery,
	// 2 AC transitions.
	if batCount < 2 {
		t.Errorf("OnBattery called %d times, want ≥ 2", batCount)
	}
	if acCount < 2 {
		t.Errorf("OnAC called %d times, want ≥ 2", acCount)
	}
}

func TestWatchdog_NoCallbacksNoCrash(t *testing.T) {
	// Omitting both callbacks must still run cleanly — a caller might
	// only care about one transition direction.
	w := &Watchdog{Interval: 5 * time.Millisecond}
	w.detect = func() PowerState { return StateAC }

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_ = w.Run(ctx)
}
