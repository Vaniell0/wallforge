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
		func() {
			// Battery: drop every backend. mpvpaper and lwpe are the
			// expensive ones; a static swww image is already zero-cost
			// after load but StopAll handles it uniformly.
			fmt.Fprintln(os.Stderr, "wallforge watchdog: on battery — stopping backends")
			for _, e := range engine.StopAll(cfg) {
				fmt.Fprintf(os.Stderr, "wallforge watchdog: stop: %v\n", e)
			}
		},
		func() {
			// AC: re-apply the last wallpaper if we have one on record.
			entry, err := state.Load()
			if err != nil || entry.Input == "" {
				return
			}
			fmt.Fprintf(os.Stderr, "wallforge watchdog: AC — resuming %s\n", entry.Input)
			if _, err := apply.ByInput(cfg, entry.Input); err != nil {
				fmt.Fprintf(os.Stderr, "wallforge watchdog: resume: %v\n", err)
			}
		},
	)

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintln(os.Stderr, "wallforge: battery watchdog running (15s poll)")
	return w.Run(ctx)
}
