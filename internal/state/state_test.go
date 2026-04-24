package state

import (
	"os"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	now := time.Now().UTC().Truncate(time.Second)
	in := Entry{Input: "12345", AppliedAt: now}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Input != in.Input {
		t.Errorf("Input = %q, want %q", got.Input, in.Input)
	}
	if !got.AppliedAt.Equal(in.AppliedAt) {
		t.Errorf("AppliedAt = %v, want %v", got.AppliedAt, in.AppliedAt)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	got, err := Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if got.Input != "" {
		t.Errorf("expected empty Entry, got Input=%q", got.Input)
	}
}

func TestSaveOverwrites(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := Save(Entry{Input: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(Entry{Input: "second"}); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Input != "second" {
		t.Errorf("Input = %q, want second", got.Input)
	}
}

func TestLoadMalformed(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Seed the parent directory via a successful Save, then clobber
	// the file with garbage.
	if err := Save(Entry{Input: "seed"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte("not json at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected error on malformed state, got nil")
	}
}
