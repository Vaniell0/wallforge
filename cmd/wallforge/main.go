// Command wallforge is a unified wallpaper manager for Hyprland.
//
// Usage:
//
//	wallforge apply <path>        set wallpaper from image/video/scene
//	wallforge workshop <id|url>   fetch Steam Workshop item and apply it
//	wallforge stop                kill running wallpaper backends
//	wallforge version             print version
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vaniello/wallforge/internal/engine"
	"github.com/vaniello/wallforge/internal/workshop"
)

const version = "0.1.0-alpha"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "apply":
		if err := cmdApply(os.Args[2:]); err != nil {
			die(err)
		}
	case "workshop":
		if err := cmdWorkshop(os.Args[2:]); err != nil {
			die(err)
		}
	case "stop":
		if err := cmdStop(); err != nil {
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

func cmdWorkshop(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("workshop: expected 1 argument (workshop id or url), got %d", len(args))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dl := workshop.NewProxyDownloader(workshop.DefaultCacheDir())
	fmt.Fprintf(os.Stderr, "wallforge: fetching workshop item %s...\n", args[0])
	dir, err := dl.Download(ctx, args[0])
	if err != nil {
		return fmt.Errorf("workshop download: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wallforge: extracted to %s\n", dir)
	return cmdApply([]string{dir})
}

func cmdApply(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("apply: expected 1 argument (path), got %d", len(args))
	}
	target, err := engine.Detect(args[0])
	if err != nil {
		return err
	}
	backend, err := engine.Select(target)
	if err != nil {
		return err
	}
	if target.Project != nil {
		fmt.Printf("wallforge: %s (workshop %q) → %s\n",
			target.Kind, target.Project.Title, backend.Name())
	} else {
		fmt.Printf("wallforge: %s → %s\n", target.Kind, backend.Name())
	}
	return backend.Apply(target.Path)
}

func cmdStop() error {
	errs := engine.StopAll()
	if len(errs) == 0 {
		return nil
	}
	// Report the first non-trivial error but keep going with others.
	return fmt.Errorf("stop: %d backend(s) reported errors: %v", len(errs), errs)
}

func usage() {
	fmt.Fprintln(os.Stderr, `wallforge — unified wallpaper manager

Usage:
  wallforge apply <path>         set wallpaper (image/video/scene)
  wallforge workshop <id|url>    fetch Steam Workshop item and apply it
  wallforge stop                 kill running wallpaper backends
  wallforge version              print version`)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "wallforge:", err)
	os.Exit(1)
}
