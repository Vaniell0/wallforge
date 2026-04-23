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

# Steam Workshop (requires linux-wallpaperengine on PATH for scene items):
./wallforge workshop 1234567890
./wallforge workshop "https://steamcommunity.com/sharedfiles/filedetails/?id=1234567890"
```

## Supported content

| Kind   | Detected by            | Backend                   | Status |
|--------|------------------------|---------------------------|--------|
| image  | extension              | swww (`awww` on nixpkgs)  | done   |
| video  | extension              | mpvpaper                  | done   |
| scene  | dir with `project.json`| linux-wallpaperengine     | done   |

## License

Apache 2.0.
