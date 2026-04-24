# Wallforge

Unified wallpaper manager for Hyprland. One CLI over `swww` / `mpvpaper` /
`linux-wallpaperengine` with Steam Workshop support for Wallpaper Engine
content.

## Status

`0.1.0-alpha` — image wallpapers via swww work. Video and Steam Workshop
support is in progress.

## Quick start

```bash
nix build
./result/bin/wallforge apply ~/Pictures/Wallpapers/my.jpg

# Steam Workshop (requires a Wallpaper Engine license and a subscription
# in Steam — wallforge reads Steam's local cache directly):
./result/bin/wallforge list
./result/bin/wallforge apply 3682370294

# Local web-UI (browse subscriptions, preview thumbnails, apply/stop):
./result/bin/wallforge serve            # http://127.0.0.1:7777
```

## Configuration

`wallforge config` prints the path and the effective values. Create
`$XDG_CONFIG_HOME/wallforge/config.json` (usually
`~/.config/wallforge/config.json`) to override any field, e.g.:

```json
{
  "steam": { "root": "/mnt/games/SteamLibrary" },
  "swww":  { "transition": "wipe", "duration": "0.8" },
  "wpe":   { "fps": 60, "silent": false }
}
```

## Supported content

| Kind   | Detected by            | Backend                   | Status |
|--------|------------------------|---------------------------|--------|
| image  | extension              | swww (`awww` on nixpkgs)  | done   |
| video  | extension              | mpvpaper                  | done   |
| scene  | dir with `project.json`| linux-wallpaperengine     | done   |

## License

Apache 2.0.
