// Command wallforge is a unified wallpaper manager for Hyprland.
//
// Usage:
//
//	wallforge apply <path>    set wallpaper from image/video/scene
//	wallforge stop            kill running wallpaper backends
//	wallforge version         print version
package main

import (
	"fmt"
	"os"

	"github.com/vaniello/wallforge/internal/engine"
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

func cmdApply(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("apply: expected 1 argument (path), got %d", len(args))
	}
	path := args[0]

	kind, err := engine.Detect(path)
	if err != nil {
		return err
	}
	backend, err := engine.Select(kind)
	if err != nil {
		return err
	}
	fmt.Printf("wallforge: %s → %s\n", kind, backend.Name())
	return backend.Apply(path)
}

func cmdStop() error {
	// For MVP just try swww; later iterate all known backends.
	return engine.NewSwww().Stop()
}

func usage() {
	fmt.Fprintln(os.Stderr, `wallforge — unified wallpaper manager

Usage:
  wallforge apply <path>    set wallpaper (image/video/scene)
  wallforge stop            kill running wallpaper backends
  wallforge version         print version`)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "wallforge:", err)
	os.Exit(1)
}
