// Package engine defines the Backend interface and dispatches wallpaper
// requests to the appropriate backend based on content type.
package engine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Vaniell0/wallforge/internal/config"
	"github.com/Vaniell0/wallforge/internal/workshop"
)

// Kind classifies wallpaper content by rendering backend.
type Kind int

const (
	KindUnknown Kind = iota
	KindImage        // png, jpg, jpeg, webp, bmp → swww
	KindVideo        // mp4, webm, mkv, gif → mpvpaper
	KindScene        // linux-wallpaperengine project dir (scene/web WE format)
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

// Target describes a concrete wallpaper request after content detection.
//
// Path is the path that should be handed to the backend — this can differ
// from the user-supplied input when a WE project directory is resolved to
// an inner file (video-type workshop items).
type Target struct {
	Kind    Kind
	Path    string
	Project *workshop.Project // nil unless input was a WE project directory
}

// Backend renders wallpapers of a specific Kind.
type Backend interface {
	Name() string
	Apply(path string) error
	Stop() error
}

// Detect classifies a filesystem path and resolves it to a Target.
//
// A directory is inspected for project.json (Wallpaper Engine item). If
// present, the WE `type` field determines the Kind and, for video items,
// the Target.Path is pointed at the inner asset. A plain file is
// classified by extension.
func Detect(path string) (Target, error) {
	info, err := osStat(path)
	if err != nil {
		return Target{}, err
	}
	if info.IsDir() {
		return detectDir(path)
	}
	return detectFile(path)
}

func detectDir(path string) (Target, error) {
	proj, err := workshop.ParseDir(path)
	if err != nil {
		return Target{}, fmt.Errorf("parse workshop project: %w", err)
	}
	if proj == nil {
		return Target{}, fmt.Errorf("directory %q has no project.json — not a Wallpaper Engine item", path)
	}
	kind, err := kindFromWorkshopType(proj.Type)
	if err != nil {
		return Target{}, err
	}
	return Target{Kind: kind, Path: proj.EffectivePath(), Project: proj}, nil
}

func detectFile(path string) (Target, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp":
		return Target{Kind: KindImage, Path: path}, nil
	case ".mp4", ".webm", ".mkv", ".mov", ".gif":
		return Target{Kind: KindVideo, Path: path}, nil
	}
	return Target{}, fmt.Errorf("unsupported file: %s", path)
}

func kindFromWorkshopType(t workshop.Type) (Kind, error) {
	switch t {
	case workshop.TypeScene, workshop.TypeWeb:
		return KindScene, nil
	case workshop.TypeVideo:
		return KindVideo, nil
	case workshop.TypeImage:
		return KindImage, nil
	case workshop.TypeApplication:
		return 0, fmt.Errorf("workshop type %q not supported (native app)", t)
	}
	return 0, fmt.Errorf("unknown workshop type %q", t)
}

// Select returns the backend that handles the given Target, configured
// from cfg. Returns an error if no backend supports its Kind yet.
func Select(t Target, cfg config.Config) (Backend, error) {
	switch t.Kind {
	case KindImage:
		return NewSwww(cfg.Swww), nil
	case KindVideo:
		return NewMpvpaper(cfg.Mpvpaper), nil
	case KindScene:
		return NewWallpaperEngine(cfg.Wpe), nil
	}
	return nil, fmt.Errorf("no backend for kind %s", t.Kind)
}

// StopAll attempts to stop every backend. Errors from individual backends
// are collected but do not abort the sequence.
func StopAll(cfg config.Config) []error {
	var errs []error
	for _, b := range []Backend{
		NewSwww(cfg.Swww),
		NewMpvpaper(cfg.Mpvpaper),
		NewWallpaperEngine(cfg.Wpe),
	} {
		if err := b.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", b.Name(), err))
		}
	}
	return errs
}

// StopOthers stops every backend except the one identified by keep.
// Used before Apply so a new image doesn't render behind a still-live
// mpvpaper / lwpe layer surface. Errors are swallowed — a backend that
// isn't running is expected to "fail" to stop, and we don't want to
// block a successful apply over it.
func StopOthers(keep Backend, cfg config.Config) {
	for _, b := range []Backend{
		NewSwww(cfg.Swww),
		NewMpvpaper(cfg.Mpvpaper),
		NewWallpaperEngine(cfg.Wpe),
	} {
		if b.Name() == keep.Name() {
			continue
		}
		_ = b.Stop()
	}
}
