// Package config loads the wallforge user configuration from
// $XDG_CONFIG_HOME/wallforge/config.json (falling back to
// ~/.config/wallforge/config.json). Missing files fall back to defaults;
// partial configs are merged over the defaults.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Config holds user-visible settings for every subsystem. Fields use
// pointer/zero-value semantics where we want to distinguish "unset" from
// "set to default" — but at this scale plain values + Default() merging
// are simpler, so we just fill the struct with defaults first and then
// overlay whatever the file provides.
type Config struct {
	Steam    SteamConfig    `json:"steam"`
	Swww     SwwwConfig     `json:"swww"`
	Mpvpaper MpvpaperConfig `json:"mpvpaper"`
	Wpe      WpeConfig      `json:"wpe"`
	Library  LibraryConfig  `json:"library"`
	Watchdog WatchdogConfig `json:"watchdog"`
}

type SteamConfig struct {
	// Root overrides auto-detection of the Steam install (useful for
	// flatpak users, shared libraries on other drives, etc). When empty,
	// wallforge searches standard locations.
	Root string `json:"root"`
}

type SwwwConfig struct {
	Transition string `json:"transition"`
	Duration   string `json:"duration"`
}

type MpvpaperConfig struct {
	Target  string `json:"target"`
	MpvOpts string `json:"mpv_opts"`
}

type WpeConfig struct {
	Fps    int    `json:"fps"`
	Silent bool   `json:"silent"`
	Screen string `json:"screen"`
}

// LibraryConfig tells the scanner which local directories to index as
// wallpaper sources. Leading "~/" is expanded at scan time.
type LibraryConfig struct {
	Roots []string `json:"roots"`
}

// WatchdogConfig tunes the auto-pause behaviour. Battery → pause is
// always on; PauseOnPowerSaver opts the watchdog into reacting to ppd
// profile changes too, so a manual switch to "power-saver" without
// unplugging still stops video/scene backends.
type WatchdogConfig struct {
	PauseOnPowerSaver bool `json:"pause_on_power_saver"`
}

// Default returns the built-in configuration. Every field here is also
// the fallback when the user's config file omits it.
func Default() Config {
	return Config{
		Swww: SwwwConfig{
			Transition: "grow",
			Duration:   "1.5",
		},
		Mpvpaper: MpvpaperConfig{
			Target:  "*",
			MpvOpts: "no-audio --loop-file=inf --panscan=1.0",
		},
		Wpe: WpeConfig{
			Fps:    30,
			Silent: true,
		},
		Library: LibraryConfig{
			Roots: []string{"~/Pictures/Wallpapers"},
		},
		Watchdog: WatchdogConfig{
			PauseOnPowerSaver: true,
		},
	}
}

// Path returns the location of the user config file.
// $XDG_CONFIG_HOME/wallforge/config.json, or ~/.config/wallforge/...
// when that env var is empty.
func Path() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "wallforge", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "wallforge.json")
	}
	return filepath.Join(home, ".config", "wallforge", "config.json")
}

// Load reads the config file if present and merges it over Default().
// Missing file is not an error; malformed JSON is.
func Load() (Config, error) {
	return LoadFrom(Path())
}

// LoadFrom is Load with an explicit path — used by tests.
func LoadFrom(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// Marshal returns the pretty-printed JSON representation of c.
func (c Config) Marshal() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}
