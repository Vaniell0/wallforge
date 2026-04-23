// Command wallforge is a unified wallpaper manager for Hyprland.
//
// Usage:
//
//	wallforge apply <path|id>    set wallpaper from file, dir or workshop ID
//	wallforge list               list subscribed Wallpaper Engine items
//	wallforge config             show config path and effective values
//	wallforge stop               kill running wallpaper backends
//	wallforge version            print version
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"text/tabwriter"

	"github.com/vaniello/wallforge/internal/config"
	"github.com/vaniello/wallforge/internal/engine"
	"github.com/vaniello/wallforge/internal/steam"
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
	input := args[0]

	// Pure-numeric argument is interpreted as a Steam Workshop ID and
	// resolved against the local Steam install.
	path := input
	if isNumericID(input) {
		resolved, err := steam.Resolve(cfg.Steam.Root, input)
		if err != nil {
			return err
		}
		path = resolved
	}

	target, err := engine.Detect(path)
	if err != nil {
		return err
	}
	backend, err := engine.Select(target, cfg)
	if err != nil {
		return err
	}
	if target.Project != nil {
		fmt.Printf("wallforge: %s (%q) → %s\n", target.Kind, target.Project.Title, backend.Name())
	} else {
		fmt.Printf("wallforge: %s → %s\n", target.Kind, backend.Name())
	}
	return backend.Apply(target.Path)
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

func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func usage() {
	fmt.Fprintln(os.Stderr, `wallforge — unified wallpaper manager

Usage:
  wallforge apply <path|id>    set wallpaper (path or Steam Workshop ID)
  wallforge list               list subscribed Wallpaper Engine items
  wallforge config             show config path and effective values
  wallforge stop               kill running wallpaper backends
  wallforge version            print version`)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "wallforge:", err)
	os.Exit(1)
}
