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

func TestPendingRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := SavePending(Entry{Input: "queued.png"}); err != nil {
		t.Fatalf("SavePending: %v", err)
	}
	got, err := LoadPending()
	if err != nil || got.Input != "queued.png" {
		t.Fatalf("LoadPending = %v, %v; want Input=queued.png", got, err)
	}
}

func TestPendingDoesNotTouchLast(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := Save(Entry{Input: "real.png"}); err != nil {
		t.Fatal(err)
	}
	if err := SavePending(Entry{Input: "queued.png"}); err != nil {
		t.Fatal(err)
	}
	last, _ := Load()
	if last.Input != "real.png" {
		t.Errorf("last.json got clobbered: Input=%q, want real.png", last.Input)
	}
}

func TestConsumePendingClears(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := SavePending(Entry{Input: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := ConsumePending()
	if err != nil || got.Input != "x" {
		t.Fatalf("ConsumePending = (%v, %v), want Input=x", got, err)
	}
	again, err := LoadPending()
	if err != nil {
		t.Fatal(err)
	}
	if again.Input != "" {
		t.Errorf("pending not cleared after consume: %q", again.Input)
	}
}

func TestConsumePendingEmpty(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// No pending → ConsumePending returns zero entry without error.
	got, err := ConsumePending()
	if err != nil {
		t.Fatalf("ConsumePending: %v", err)
	}
	if got.Input != "" {
		t.Errorf("expected empty, got %q", got.Input)
	}
}

func TestClearPendingMissing(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// ENOENT is not an error — the file may never have been written.
	if err := ClearPending(); err != nil {
		t.Errorf("ClearPending on missing file: %v", err)
	}
}
