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
	// BatteryMpvOpts is appended to MpvOpts when the watchdog reports
	// LowPower (AC + ppd power-saver, by default). Empty = leave
	// MpvOpts unchanged in LowPower. mpv processes options
	// left-to-right so later flags override earlier ones — meaning
	// "--cache-secs=2" in BatteryMpvOpts wins over a "--cache-secs=10"
	// in MpvOpts. The user's base opts (e.g. "--mute") survive.
	BatteryMpvOpts string `json:"battery_mpv_opts"`
	// Nice is the scheduling priority adjustment applied after the
	// mpvpaper process is spawned. Positive values lower its CPU
	// priority so the foreground stays snappy; 0 disables the call.
	Nice int `json:"nice"`
}

type WpeConfig struct {
	Fps    int    `json:"fps"`
	Silent bool   `json:"silent"`
	Screen string `json:"screen"`
	// FpsBattery overrides Fps in LowPower mode. 0 = keep Fps.
	FpsBattery int `json:"fps_battery"`
	// Nice — see MpvpaperConfig.Nice. lwpe is the heaviest backend
	// (full GL scene); a polite default keeps a CPU-bound build
	// from stuttering when a wallpaper happens to spike.
	Nice int `json:"nice"`
}

// LibraryConfig tells the scanner which local directories to index as
// wallpaper sources. Leading "~/" is expanded at scan time.
type LibraryConfig struct {
	Roots []string `json:"roots"`
}

// WatchdogConfig tunes the auto-pause behaviour. Battery → ModePaused
// is hardcoded; PowerSaverPolicy chooses what to do on AC + ppd
// power-saver. Allowed values:
//
//	"reduce"  — drop into LowPower mode (battery_mpv_opts, fps_battery)
//	"pause"   — full stop, like on battery
//	"ignore"  — keep running at Normal quality
//
// Anything else falls back to "reduce".
type WatchdogConfig struct {
	PowerSaverPolicy string `json:"power_saver_policy"`
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
			Target: "*",
			// Base opts always apply. Battery opts are appended in
			// LowPower — kept slim because they only add tweaks, not
			// duplicate the base. mpv reads left-to-right so later
			// flags win when keys collide.
			MpvOpts:        "no-audio --loop-file=inf --panscan=1.0",
			BatteryMpvOpts: "--hwdec=auto --cache-secs=2 --video-sync=display-vdrop",
			Nice:           10,
		},
		Wpe: WpeConfig{
			Fps:        30,
			Silent:     true,
			FpsBattery: 15,
			Nice:       10,
		},
		Library: LibraryConfig{
			Roots: []string{"~/Pictures/Wallpapers"},
		},
		Watchdog: WatchdogConfig{
			PowerSaverPolicy: "reduce",
		},
	}
}

// ForLowPower returns a Config with the LowPower-mode overrides
// applied. BatteryMpvOpts is *appended* to MpvOpts so the user's base
// flags survive (e.g. "--mute" stays in effect even when battery opts
// add hwdec). FpsBattery replaces Fps wholesale because lwpe's --fps
// is a single value, not composable. Empty / zero overrides leave
// the original untouched.
func (c Config) ForLowPower() Config {
	out := c
	if c.Mpvpaper.BatteryMpvOpts != "" {
		if c.Mpvpaper.MpvOpts == "" {
			out.Mpvpaper.MpvOpts = c.Mpvpaper.BatteryMpvOpts
		} else {
			out.Mpvpaper.MpvOpts = c.Mpvpaper.MpvOpts + " " + c.Mpvpaper.BatteryMpvOpts
		}
	}
	if c.Wpe.FpsBattery > 0 {
		out.Wpe.Fps = c.Wpe.FpsBattery
	}
	return out
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
