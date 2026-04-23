package workshop

import (
	"os"
	"path/filepath"
	"testing"
)

func writeJSON(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseDir_SceneType(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "project.json"), `{
	    "title": "Aurora",
	    "type": "scene",
	    "file": "scene.pkg",
	    "workshopid": "1234567890",
	    "preview": "preview.jpg"
	}`)

	p, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p == nil {
		t.Fatal("expected project, got nil")
	}
	if p.Type != TypeScene {
		t.Errorf("Type = %q, want scene", p.Type)
	}
	if p.WorkshopID != "1234567890" {
		t.Errorf("WorkshopID = %q", p.WorkshopID)
	}
	if got := p.EffectivePath(); got != dir {
		t.Errorf("EffectivePath for scene = %q, want %q", got, dir)
	}
}

func TestParseDir_VideoType(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "project.json"), `{
	    "title": "Rainfall",
	    "type": "Video",
	    "file": "rain.mp4"
	}`)

	p, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != TypeVideo {
		t.Errorf("Type should be lowercased to video, got %q", p.Type)
	}
	want := filepath.Join(dir, "rain.mp4")
	if got := p.EffectivePath(); got != want {
		t.Errorf("EffectivePath = %q, want %q", got, want)
	}
}

func TestParseDir_Missing(t *testing.T) {
	p, err := ParseDir(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error for missing project.json, got %v", err)
	}
	if p != nil {
		t.Errorf("expected nil project, got %+v", p)
	}
}

func TestParseFile_Malformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "project.json")
	writeJSON(t, path, `{ not json }`)
	if _, err := ParseFile(path); err == nil {
		t.Error("expected parse error on malformed json")
	}
}
