package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_PlainFiles(t *testing.T) {
	dir := t.TempDir()

	img := filepath.Join(dir, "wall.jpg")
	if err := os.WriteFile(img, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatal(err)
	}
	vid := filepath.Join(dir, "wall.mp4")
	if err := os.WriteFile(vid, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path string
		want Kind
	}{
		{img, KindImage},
		{vid, KindVideo},
	}
	for _, c := range cases {
		got, err := Detect(c.path)
		if err != nil {
			t.Errorf("Detect(%s): %v", c.path, err)
			continue
		}
		if got.Kind != c.want {
			t.Errorf("Detect(%s).Kind = %s, want %s", c.path, got.Kind, c.want)
		}
		if got.Path != c.path {
			t.Errorf("Detect(%s).Path = %s, want %s", c.path, got.Path, c.path)
		}
		if got.Project != nil {
			t.Errorf("Detect(%s).Project should be nil for plain files", c.path)
		}
	}
}

func TestDetect_WorkshopScene(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "project.json"),
		[]byte(`{"type":"scene","file":"scene.pkg","title":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindScene {
		t.Errorf("Kind = %s, want scene", got.Kind)
	}
	if got.Path != dir {
		t.Errorf("Path = %s, want %s (scene → dir)", got.Path, dir)
	}
	if got.Project == nil {
		t.Error("Project should be populated for WE items")
	}
}

func TestDetect_WorkshopVideo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "project.json"),
		[]byte(`{"type":"video","file":"rain.mp4"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindVideo {
		t.Errorf("Kind = %s, want video", got.Kind)
	}
	want := filepath.Join(dir, "rain.mp4")
	if got.Path != want {
		t.Errorf("Path = %s, want %s (video → inner file)", got.Path, want)
	}
}

func TestDetect_WorkshopWeb(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "project.json"),
		[]byte(`{"type":"web","file":"index.html"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindScene {
		t.Errorf("Kind = %s, want scene (web is also handled by lwpe)", got.Kind)
	}
}

func TestDetect_BareDirFails(t *testing.T) {
	if _, err := Detect(t.TempDir()); err == nil {
		t.Error("expected error for directory without project.json")
	}
}

func TestSelect_AllKinds(t *testing.T) {
	cases := []struct {
		kind Kind
		want string
	}{
		{KindImage, "swww"},
		{KindVideo, "mpvpaper"},
		{KindScene, "linux-wallpaperengine"},
	}
	for _, c := range cases {
		b, err := Select(Target{Kind: c.kind})
		if err != nil {
			t.Errorf("Select(%s): unexpected error %v", c.kind, err)
			continue
		}
		if b.Name() != c.want {
			t.Errorf("Select(%s).Name = %q, want %q", c.kind, b.Name(), c.want)
		}
	}
}

func TestDetect_UnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(bad, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Detect(bad); err == nil {
		t.Error("expected error for .txt")
	}
}
