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
	w := watchdog.New(
		15*time.Second,
		cfg.Watchdog.PauseOnPowerSaver,
		func(reason string) {
			// Pause: drop every backend. mpvpaper and lwpe are the
			// expensive ones; static swww costs nothing after load
			// but StopAll handles it uniformly.
			fmt.Fprintf(os.Stderr, "wallforge watchdog: pause (%s) — stopping backends\n", reason)
			for _, e := range engine.StopAll(cfg) {
				fmt.Fprintf(os.Stderr, "wallforge watchdog: stop: %v\n", e)
			}
		},
		func() {
			// Resume: re-apply the last wallpaper if we have one on record.
			entry, err := state.Load()
			if err != nil || entry.Input == "" {
				return
			}
			fmt.Fprintf(os.Stderr, "wallforge watchdog: resume — restoring %s\n", entry.Input)
			if _, err := apply.ByInput(cfg, entry.Input); err != nil {
				fmt.Fprintf(os.Stderr, "wallforge watchdog: resume: %v\n", err)
			}
		},
	)

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	saver := "ignored"
	if cfg.Watchdog.PauseOnPowerSaver {
		saver = "respected"
	}
	fmt.Fprintf(os.Stderr, "wallforge: watchdog running (15s poll, ppd power-saver %s)\n", saver)
	return w.Run(ctx)
}
