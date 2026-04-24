# Wallforge

Unified wallpaper manager for Hyprland. One CLI, one web-UI, one story for
static images, video wallpapers, and Steam Workshop Wallpaper Engine scenes.

```
wallforge apply ~/Pictures/Wallpapers/island.png     # static via swww
wallforge apply ~/Videos/sakura.mp4                  # video via mpvpaper
wallforge apply 3682370294                           # Steam Workshop WE item
```

Stdlib-first Go. One static binary. No runtime other than the wallpaper
engines themselves (`swww`, `mpvpaper`, `linux-wallpaperengine`). The web-UI
is served from the same binary — HTML/JS/CSS are `//go:embed`ed.

## Status

`0.1.0-alpha` — image / video / scene backends work end-to-end. Steam
Workshop integration reads the local Steam install (no proxy services,
no DMCA-bait). Web-UI, per-workspace daemon, battery watchdog, shell
completion all wired. [Roadmap](#roadmap) for what's next.

## Who it's for

- **Hyprland / Wayland users** who want one CLI over the three common
  wallpaper backends instead of three bespoke scripts.
- **Wallpaper Engine fans on Linux** — Steam Workshop subscriptions render
  natively via [`linux-wallpaperengine`](https://github.com/Almamu/linux-wallpaperengine),
  no Proton, no Windows.
- **NixOS users** — flake + Home Manager module + overlay ship in-tree.
  The `linux-wallpaperengine` Nix derivation is in `nix/` and builds end-to-end,
  including the CEF pre-fetch.

Not trying to replace Plasma's or GNOME's native wallpaper managers — those
are great on their own DEs.

## Install

### Nix / NixOS (flake + Home Manager)

```nix
# ~/.config/home-manager/flake.nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager.url = "github:nix-community/home-manager";
    wallforge.url = "github:Vaniell0/wallforge";
    wallforge.inputs.nixpkgs.follows = "nixpkgs";
  };
  outputs = { self, nixpkgs, home-manager, wallforge, ... }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs {
        inherit system;
        overlays = [ wallforge.overlays.default ];
      };
    in {
      homeConfigurations.you = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        modules = [
          wallforge.homeManagerModules.default
          ({ ... }: {
            programs.wallforge = {
              enable = true;
              serve.enable = true;       # web-UI at http://127.0.0.1:7777
              resume.enable = true;      # re-apply last wallpaper on login
              workspace.enable = true;   # per-workspace daemon
              watchdog.enable = true;    # stop backends on battery
            };
          })
        ];
      };
    };
}
```

Every `*.enable` option is independent — start with `enable = true` alone,
layer on the others as you want them.

Config knobs live under `programs.wallforge.settings`:

```nix
programs.wallforge.settings = {
  swww.transition = "wipe";
  swww.duration   = "0.8";
  wpe.fps         = 20;
  library.roots   = [ "~/Pictures/Wallpapers" "/mnt/art" ];
};
```

### Arch (AUR)

```
git clone https://aur.archlinux.org/wallforge.git
cd wallforge && makepkg -si
```

Or use the in-tree `PKGBUILD` directly from this repo: `pkg/arch/PKGBUILD`.

### From source

```bash
git clone https://github.com/Vaniell0/wallforge.git
cd wallforge
go build -o wallforge ./cmd/wallforge
./wallforge version
```

Requires Go 1.25+. Runtime deps: `swww` (or its nixpkgs alias `awww`) and
— for video / scene content — `mpvpaper` and `linux-wallpaperengine`.

## CLI

```
wallforge apply <path|id>                       set wallpaper
wallforge shuffle [flags] [ids...]              rotate a playlist
wallforge serve [--addr=127.0.0.1:7777]         start the web-UI
wallforge resume                                re-apply the last wallpaper
wallforge workspace bind <ws> <path|id>         bind a wallpaper to a workspace
wallforge workspace unbind <ws>                 remove a workspace binding
wallforge workspace list                        show bindings
wallforge workspace daemon                      run the per-workspace daemon
wallforge watchdog                              battery-aware backend control
wallforge list                                  subscribed WE items
wallforge config                                config path + effective values
wallforge stop                                  stop running backends
wallforge completion <bash|zsh|fish>            print completion script
```

**`apply`** auto-detects the content type:

- File with an image extension → `swww`
- File with a video extension → `mpvpaper`
- Directory with a `project.json` → `linux-wallpaperengine` (scene/web)
  or `mpvpaper` (video WE item)
- Bare number → treated as a Steam Workshop ID, resolved against the
  local Steam cache

**`shuffle`** picks its playlist from explicit IDs, or from every
subscription of a given WE type:

```bash
wallforge shuffle --interval=30m --type=video
wallforge shuffle 1059450186 1116273880 --interval=5m --random=true
```

**`workspace`** bindings persist in
`$XDG_STATE_HOME/wallforge/workspaces.json`. The daemon watches
Hyprland's `.socket2` event stream and reloads the bindings on every
event, so editing bindings while the daemon runs is safe.

## Web-UI

`wallforge serve` on `127.0.0.1:7777` exposes:

| Path           | Method | Shape                                           |
|----------------|--------|-------------------------------------------------|
| `/`            | GET    | SPA                                             |
| `/static/*`    | GET    | HTML / JS / CSS (embedded in the binary)        |
| `/api/items`   | GET    | Steam Workshop subscriptions                    |
| `/api/library` | GET    | Local library items scanned from `library.roots`|
| `/api/status`  | GET    | `{last_applied: "..."}`                         |
| `/api/apply`   | POST   | `{input: "<id|path>"}`                          |
| `/api/stop`    | POST   | kill all backends                               |
| `/preview/{id}`| GET    | thumbnail for a single item                     |

No auth. The bind defaults to `127.0.0.1` because there is no auth. If
you need remote access, front it with your own reverse proxy and add
auth there.

## Config

`$XDG_CONFIG_HOME/wallforge/config.json`, generated by HM if you set
`programs.wallforge.settings`. Full schema:

```json
{
  "steam": {
    "root": ""
  },
  "swww": {
    "transition": "grow",
    "duration": "1.5"
  },
  "mpvpaper": {
    "target": "*",
    "mpv_opts": "no-audio --loop-file=inf --panscan=1.0"
  },
  "wpe": {
    "fps": 30,
    "silent": true,
    "screen": ""
  },
  "library": {
    "roots": ["~/Pictures/Wallpapers"]
  }
}
```

Every field is optional — the loader merges the file over `Default()`,
so any key you omit falls back to built-in defaults.

## How it compares

|                            | wallforge      | swww        | hyprpaper   | WE via Proton |
|----------------------------|----------------|-------------|-------------|---------------|
| Static images              | ✓              | ✓           | ✓           | ✓             |
| Video wallpapers           | ✓ via mpvpaper | ✗           | ✗           | ✓             |
| Scene / web WE content     | ✓ via lwpe     | ✗           | ✗           | ✓             |
| Steam Workshop integration | ✓ local cache  | ✗           | ✗           | native client |
| Per-workspace bindings     | ✓              | ✗           | ✓           | ✗             |
| Web-UI                     | ✓              | ✗           | ✗           | ✗             |
| Battery-aware              | ✓ watchdog     | ✗           | ✗           | — (emulator)  |
| NixOS flake + HM module    | ✓              | nixpkgs only| nixpkgs only| ✗             |
| Single static binary       | ✓              | ✓           | ✓           | —             |

The Wayland-native path (wallforge + swww + mpvpaper + lwpe) avoids
Proton overhead and works on multi-GPU / Wayland-only setups where the
Proton-wrapped Windows WE has issues.

## Roadmap

- [x] swww backend (images)
- [x] mpvpaper backend (video)
- [x] linux-wallpaperengine backend (scene / web)
- [x] Steam Workshop local resolver
- [x] Web-UI (`serve`)
- [x] Library index (local files merged with Workshop in the UI)
- [x] Per-workspace daemon (Hyprland IPC)
- [x] Battery watchdog
- [x] `resume` + persisted last-applied
- [x] Shell completion
- [x] Home Manager module + nixpkgs overlay
- [x] Arch PKGBUILD
- [ ] Per-monitor bindings (currently the backend handles the first
      active output)
- [ ] MPRIS / UPower DBus subscribers to replace filesystem polls
- [ ] GitHub Actions CI (go test + nix build on push)

## Contributing

Tests are `go test ./...` — keep the full suite green. The project is
deliberately stdlib-first; new dependencies need justification in the PR
description.

## License

Apache 2.0. See [`LICENSE`](LICENSE).
