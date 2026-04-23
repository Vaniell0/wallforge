# Wallforge

Unified wallpaper manager for Hyprland. One CLI over `swww` / `mpvpaper` /
`linux-wallpaperengine` with Steam Workshop support for Wallpaper Engine
content.

## Status

`0.1.0-alpha` — image wallpapers via swww work. Video and Steam Workshop
support is in progress.

## Quick start

```bash
nix develop
go build ./cmd/wallforge
./wallforge apply ~/Pictures/Wallpapers/my.jpg
```

## Supported content

| Kind   | Detected by            | Backend                   | Status |
|--------|------------------------|---------------------------|--------|
| image  | extension              | swww (`awww` on nixpkgs)  | done   |
| video  | extension              | mpvpaper                  | TODO   |
| scene  | dir with `project.json`| linux-wallpaperengine     | TODO   |

## License

Apache 2.0.
