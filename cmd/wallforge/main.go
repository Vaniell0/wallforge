// Command wallforge is a unified wallpaper manager for Hyprland.
//
// Usage:
//
//	wallforge apply <path|id>                set wallpaper
//	wallforge shuffle [flags] [ids...]       rotate through a playlist
//	wallforge serve [--addr=...]             start the local web-UI
//	wallforge list                           list subscribed WE items
//	wallforge config                         show config + effective values
//	wallforge stop                           kill running backends
//	wallforge version                        print version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/vaniello/wallforge/internal/apply"
	"github.com/vaniello/wallforge/internal/config"
	"github.com/vaniello/wallforge/internal/engine"
	"github.com/vaniello/wallforge/internal/steam"
	"github.com/vaniello/wallforge/internal/webui"
)

const version = "0.1.0-alpha"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		die(fmt.Errorf("load config: %w", err))
	}
	switch os.Args[1] {
	case "apply":
		if err := cmdApply(cfg, os.Args[2:]); err != nil {
			die(err)
		}
	case "shuffle":
		if err := cmdShuffle(cfg, os.Args[2:]); err != nil {
			die(err)
		}
	case "serve":
		if err := cmdServe(cfg, os.Args[2:]); err != nil {
			die(err)
		}
	case "list":
		if err := cmdList(cfg); err != nil {
			die(err)
		}
	case "config":
		if err := cmdConfig(cfg); err != nil {
			die(err)
		}
	case "stop":
		if err := cmdStop(cfg); err != nil {
			die(err)
		}
	case "version", "-v", "--version":
		fmt.Println("wallforge", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func cmdApply(cfg config.Config, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("apply: expected 1 argument (path or workshop id), got %d", len(args))
	}
	return applyByInput(cfg, args[0])
}

func cmdShuffle(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("shuffle", flag.ContinueOnError)
	var (
		interval = fs.Duration("interval", 15*time.Minute, "time between wallpaper changes")
		kind     = fs.String("type", "", "filter subscriptions by WE type (video/scene/image) when no IDs given")
		random   = fs.Bool("random", true, "randomize order instead of cycling")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	ids, err := buildPlaylist(cfg, fs.Args(), *kind)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("shuffle: empty playlist (no IDs passed and no matching subscriptions)")
	}

	fmt.Fprintf(os.Stderr, "wallforge: shuffle of %d item(s), interval %s\n", len(ids), interval)

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	order := make([]int, len(ids))
	for i := range order {
		order[i] = i
	}
	if *random {
		rand.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	}

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for pos := 0; ; pos = (pos + 1) % len(order) {
		id := ids[order[pos]]
		if err := applyByInput(cfg, id); err != nil {
			// Don't abort the whole shuffle on a single broken item —
			// just log and continue to the next tick.
			fmt.Fprintf(os.Stderr, "wallforge: apply %s failed: %v\n", id, err)
		}
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\nwallforge: shuffle stopped")
			return nil
		case <-ticker.C:
		}
		// Re-shuffle when wrapping around so sequential runs don't
		// converge on the same order.
		if *random && pos+1 == len(order) {
			rand.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		}
	}
}

// buildPlaylist returns the list of workshop IDs / paths to cycle
// through. Explicit args win; otherwise we pull the subscriptions of
// the requested type.
func buildPlaylist(cfg config.Config, args []string, kind string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	items, err := steam.List(cfg.Steam.Root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, it := range items {
		if it.Project == nil {
			continue
		}
		if kind != "" && string(it.Project.Type) != kind {
			continue
		}
		ids = append(ids, it.ID)
	}
	return ids, nil
}

// applyByInput resolves the single-input apply path used by both
// `apply` and `shuffle`. Wraps apply.ByInput with human-readable
// console output.
func applyByInput(cfg config.Config, input string) error {
	res, err := apply.ByInput(cfg, input)
	if err != nil {
		return err
	}
	if res.Title != "" {
		fmt.Printf("wallforge: %s (%q) → %s\n", res.Kind, res.Title, res.Backend)
	} else {
		fmt.Printf("wallforge: %s → %s\n", res.Kind, res.Backend)
	}
	return nil
}

func cmdServe(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:7777", "address to bind (host:port)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	srv, err := webui.New(cfg, *addr)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stderr, "wallforge: web-UI on http://%s\n", *addr)
	return srv.Run(ctx)
}

func cmdList(cfg config.Config) error {
	items, err := steam.List(cfg.Steam.Root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w\n\nIs Wallpaper Engine installed and subscribed to anything? "+
				"Set steam.root in config to override the auto-detected path.", err)
		}
		return err
	}
	if len(items) == 0 {
		fmt.Println("No Wallpaper Engine workshop items subscribed.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tTITLE")
	for _, it := range items {
		typ := "?"
		title := "(no project.json)"
		if it.Project != nil {
			title = it.Project.Title
			if it.Project.Type != "" {
				typ = string(it.Project.Type)
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", it.ID, typ, title)
	}
	return w.Flush()
}

func cmdConfig(cfg config.Config) error {
	fmt.Println("path:", config.Path())
	data, err := cfg.Marshal()
	if err != nil {
		return err
	}
	fmt.Println("effective config:")
	fmt.Println(string(data))
	return nil
}

func cmdStop(cfg config.Config) error {
	errs := engine.StopAll(cfg)
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("stop: %d backend(s) reported errors: %v", len(errs), errs)
}

func usage() {
	fmt.Fprintln(os.Stderr, `wallforge — unified wallpaper manager

Usage:
  wallforge apply <path|id>
  wallforge shuffle [--interval=15m] [--type=video] [--random=true] [ids...]
  wallforge serve [--addr=127.0.0.1:7777]
  wallforge list
  wallforge config
  wallforge stop
  wallforge version

shuffle picks its playlist from explicit IDs, or from all subscriptions
filtered by --type when no IDs are given. --interval accepts Go duration
syntax (30s, 5m, 1h). serve starts the local web-UI.`)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "wallforge:", err)
	os.Exit(1)
}
