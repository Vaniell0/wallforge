package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFrom_Missing(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	def := Default()
	if cfg.Swww != def.Swww {
		t.Errorf("missing file: Swww should equal Default(), got %+v", cfg.Swww)
	}
	if cfg.Wpe.Fps != 30 {
		t.Errorf("missing file: Wpe.Fps should default to 30, got %d", cfg.Wpe.Fps)
	}
}

func TestLoadFrom_PartialOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
	  "swww": { "transition": "wipe" },
	  "wpe":  { "fps": 60 }
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Swww.Transition != "wipe" {
		t.Errorf("override not applied: got %q", cfg.Swww.Transition)
	}
	// Duration was not overridden — should keep default.
	if cfg.Swww.Duration != "1.5" {
		t.Errorf("unset field should keep default, got %q", cfg.Swww.Duration)
	}
	if cfg.Wpe.Fps != 60 {
		t.Errorf("Wpe.Fps override: got %d, want 60", cfg.Wpe.Fps)
	}
	if !cfg.Wpe.Silent {
		t.Errorf("Wpe.Silent should default to true, got false")
	}
}

func TestLoadFrom_Malformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{ not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(path); err == nil {
		t.Error("malformed JSON should fail")
	}
}

func TestPath_XdgOverridesHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x/y")
	if got, want := Path(), "/x/y/wallforge/config.json"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestPath_HomeFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	if got, want := Path(), filepath.Join(home, ".config", "wallforge", "config.json"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}
