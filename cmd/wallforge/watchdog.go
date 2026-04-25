package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Vaniell0/wallforge/internal/apply"
	"github.com/Vaniell0/wallforge/internal/config"
	"github.com/Vaniell0/wallforge/internal/engine"
	"github.com/Vaniell0/wallforge/internal/state"
	"github.com/Vaniell0/wallforge/internal/watchdog"
)

func cmdWatchdog(cfg config.Config) error {
	policy := watchdog.ParsePolicy(cfg.Watchdog.PowerSaverPolicy)

	w := watchdog.New(15*time.Second, policy, func(mode watchdog.Mode, reason string) {
		switch mode {
		case watchdog.ModePaused:
			fmt.Fprintf(os.Stderr, "wallforge watchdog: paused (%s) — stopping backends\n", reason)
			for _, e := range engine.StopAll(cfg) {
				fmt.Fprintf(os.Stderr, "wallforge watchdog: stop: %v\n", e)
			}
		case watchdog.ModeNormal, watchdog.ModeLowPower:
			// Both Normal and LowPower call apply.ByInput — apply
			// auto-detects the current mode and picks the right cfg
			// override (battery_mpv_opts / fps_battery in LowPower).
			entry, err := state.Load()
			if err != nil || entry.Input == "" {
				return
			}
			if mode == watchdog.ModeLowPower {
				fmt.Fprintf(os.Stderr, "wallforge watchdog: low-power (%s) — re-applying %s\n", reason, entry.Input)
			} else {
				fmt.Fprintf(os.Stderr, "wallforge watchdog: normal — restoring %s\n", entry.Input)
			}
			if _, err := apply.ByInput(cfg, entry.Input); err != nil {
				fmt.Fprintf(os.Stderr, "wallforge watchdog: apply: %v\n", err)
			}
		}
	})

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stderr, "wallforge: watchdog running (15s poll, power-saver policy: %s)\n", policy)
	return w.Run(ctx)
}
