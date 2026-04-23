// Package engine defines the Backend interface and dispatches wallpaper
// requests to the appropriate backend based on content type.
package engine

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Kind classifies wallpaper content by rendering backend.
type Kind int

const (
	KindUnknown Kind = iota
	KindImage        // png, jpg, jpeg, webp, bmp → swww
	KindVideo        // mp4, webm, mkv, gif → mpvpaper
	KindScene        // linux-wallpaperengine project dir (scene/web/video WE format)
)

func (k Kind) String() string {
	switch k {
	case KindImage:
		return "image"
	case KindVideo:
		return "video"
	case KindScene:
		return "scene"
	default:
		return "unknown"
	}
}

// Backend renders wallpapers of a specific Kind.
type Backend interface {
	Name() string
	Apply(path string) error
	Stop() error
}

// Detect classifies a filesystem path by its extension or contents.
// A directory containing project.json is treated as a WE scene.
func Detect(path string) (Kind, error) {
	info, err := osStat(path)
	if err != nil {
		return KindUnknown, err
	}
	if info.IsDir() {
		if fileExists(filepath.Join(path, "project.json")) {
			return KindScene, nil
		}
		return KindUnknown, fmt.Errorf("directory %q has no project.json", path)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp":
		return KindImage, nil
	case ".mp4", ".webm", ".mkv", ".mov", ".gif":
		return KindVideo, nil
	}
	return KindUnknown, fmt.Errorf("unsupported file: %s", path)
}

// Select returns the backend that handles the given Kind.
// Returns an error if no backend supports it yet.
func Select(kind Kind) (Backend, error) {
	switch kind {
	case KindImage:
		return NewSwww(), nil
	case KindVideo:
		return nil, fmt.Errorf("video backend (mpvpaper) not implemented yet")
	case KindScene:
		return nil, fmt.Errorf("scene backend (linux-wallpaperengine) not implemented yet")
	}
	return nil, fmt.Errorf("no backend for kind %s", kind)
}
