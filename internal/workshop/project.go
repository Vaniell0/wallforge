// Package workshop handles Steam Workshop content for Wallpaper Engine.
//
// Right now it only parses the project.json manifest shipped with every
// workshop item; downloading, extraction and caching will land alongside
// the linux-wallpaperengine backend.
package workshop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Type is the `type` field inside project.json. Values come from the
// Wallpaper Engine editor: scene, video, web, image, application.
type Type string

const (
	TypeScene       Type = "scene"
	TypeVideo       Type = "video"
	TypeWeb         Type = "web"
	TypeImage       Type = "image"
	TypeApplication Type = "application"
)

// Project mirrors the fields we care about from Wallpaper Engine's
// project.json. Unknown fields are ignored.
type Project struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        Type     `json:"type"`
	File        string   `json:"file"`
	Preview     string   `json:"preview"`
	WorkshopID  string   `json:"workshopid"`
	WorkshopURL string   `json:"workshopurl"`
	Tags        []string `json:"tags"`

	// Dir is set by ParseDir to the directory containing project.json so
	// callers can resolve the File field without carrying a separate path.
	Dir string `json:"-"`
}

// ParseFile reads and decodes a project.json from an explicit path.
func ParseFile(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	p.Type = Type(strings.ToLower(string(p.Type)))
	p.Dir = filepath.Dir(path)
	return &p, nil
}

// ParseDir reads dir/project.json. Returns nil, nil if the file does not
// exist — callers can use errors.Is(err, fs.ErrNotExist) on ParseFile if
// they need to distinguish missing from malformed.
func ParseDir(dir string) (*Project, error) {
	path := filepath.Join(dir, "project.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return ParseFile(path)
}

// EffectivePath returns the absolute path to the main asset referenced
// by the project (e.g. the mp4 for a video-type item). For scene/web it
// returns the project directory itself — linux-wallpaperengine consumes
// a directory, not a file.
func (p *Project) EffectivePath() string {
	if p == nil {
		return ""
	}
	switch p.Type {
	case TypeVideo, TypeImage:
		if p.File == "" {
			return p.Dir
		}
		return filepath.Join(p.Dir, p.File)
	default:
		return p.Dir
	}
}
