# Configuration reference

Wallforge reads `$XDG_CONFIG_HOME/wallforge/config.json` (typically
`~/.config/wallforge/config.json`). Every field is optional — the
loader merges your file over the built-in defaults, so omit anything
you don't want to change.

Home Manager users set the same fields under
`programs.wallforge.settings`:

```nix
programs.wallforge.settings = {
  swww.transition = "wipe";
  library.roots   = [ "~/Pictures/Wallpapers" "/mnt/art/wallpapers" ];
};
```

HM writes out the JSON and links it into `~/.config/wallforge/`.

## Full schema with defaults

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
    "mpv_opts": "no-audio --loop-file=inf --panscan=1.0",
    "battery_mpv_opts": "--hwdec=auto --cache-secs=2 --video-sync=display-vdrop",
    "nice": 10
  },
  "wpe": {
    "fps": 30,
    "silent": true,
    "screen": "",
    "fps_battery": 15,
    "nice": 10
  },
  "library": {
    "roots": ["~/Pictures/Wallpapers"]
  },
  "watchdog": {
    "power_saver_policy": "reduce"
  }
}
```

## Field-by-field

### `steam`

| Field   | Type   | Default | Notes                                          |
|---------|--------|---------|------------------------------------------------|
| `root`  | string | `""`    | Steam install path. Empty = auto-detect.       |

Auto-detect walks, in order:

1. `~/.local/share/Steam`
2. `~/.steam/steam`
3. `~/.var/app/com.valvesoftware.Steam/data/Steam` (flatpak)
4. `~/.var/app/com.valvesoftware.Steam/.local/share/Steam`

Set `steam.root` if your Steam library lives on a different drive
(`/mnt/games/SteamLibrary`, `/run/media/user/SSD/Steam`, etc.). The
Wallpaper Engine Workshop dir is always resolved relative to this
root: `<root>/steamapps/workshop/content/431960/`.

### `swww`

Forwarded verbatim to `awww img --transition-type X --transition-duration Y`.

| Field        | Type   | Default  | Notes                                              |
|--------------|--------|----------|----------------------------------------------------|
| `transition` | string | `"grow"` | Any swww transition: `simple`, `fade`, `wipe`, `grow`, `outer`, `random`. |
| `duration`   | string | `"1.5"`  | Seconds. Kept as a string because swww's flag parses `0.5` / `1.5`. |

Binaries in nixpkgs are `awww` and `awww-daemon` (swww was renamed).
On other distros the binaries are still `swww` / `swww-daemon` — symlink
one set to the other.

### `mpvpaper`

Forwarded to `mpvpaper -o <mpv_opts> <target> <path>`.

| Field              | Type   | Default                                       | Notes                                                                     |
|--------------------|--------|-----------------------------------------------|---------------------------------------------------------------------------|
| `target`           | string | `"*"`                                         | Output selector. `*` = all. Specific outputs: `eDP-1`, `DP-1`.            |
| `mpv_opts`         | string | `"no-audio --loop-file=inf --panscan=1.0"`    | Any mpv flag, space-separated. Always applied.                            |
| `battery_mpv_opts` | string | `"--hwdec=auto --cache-secs=2 --video-sync=display-vdrop"` | Appended to `mpv_opts` in LowPower mode. Empty = no extra flags. |
| `nice`             | int    | `10`                                          | Process group niceness applied after spawn. 0 = no adjustment.            |

`battery_mpv_opts` is **appended**, not substituted — your base flags
(e.g. `--mute`) survive. mpv reads options left-to-right, so colliding
keys resolve to the battery side, e.g. `--cache-secs=2` from battery
opts wins over a `--cache-secs=10` in `mpv_opts`.

Common tweaks:

```json
"mpvpaper": {
  "mpv_opts": "no-audio --loop-file=inf --panscan=1.0 --hwdec=auto",
  "battery_mpv_opts": "--cache-secs=1 --vo=null"
}
```

### `wpe` (Wallpaper Engine / `linux-wallpaperengine`)

| Field         | Type    | Default | Notes                                             |
|---------------|---------|---------|---------------------------------------------------|
| `fps`         | int     | `30`    | Target FPS at Normal mode.                        |
| `silent`      | boolean | `true`  | Mute scene audio.                                 |
| `screen`      | string  | `""`    | Output name. Empty = first output.                |
| `fps_battery` | int     | `15`    | Replaces `fps` in LowPower mode. 0 = keep `fps`.  |
| `nice`        | int     | `10`    | Process group niceness. Catches CEF helpers too.  |

`fps: 60` on AC, `fps_battery: 15` on power-saver — common split for
particle-heavy WE scenes. lwpe's `--fps` is a single scalar so this
*replaces* (not appends).

### `watchdog`

| Field                 | Type   | Default    | Notes                                                          |
|-----------------------|--------|------------|----------------------------------------------------------------|
| `power_saver_policy`  | string | `"reduce"` | What to do on AC + ppd power-saver. `reduce` / `pause` / `ignore`. |

Battery → Paused is hardcoded (always pause on battery). The policy
only affects the AC + power-saver corner:

- `reduce` (default) — drop into LowPower: re-apply with
  `battery_mpv_opts` appended and `fps_battery` substituted.
- `pause` — full stop, like on battery.
- `ignore` — keep running at Normal quality regardless of profile.

Anything else (typo, unknown value) silently falls back to `reduce`.

### `library`

| Field   | Type     | Default                     | Notes                                        |
|---------|----------|-----------------------------|----------------------------------------------|
| `roots` | string[] | `["~/Pictures/Wallpapers"]` | Directories scanned by `/api/library`.       |

Leading `~/` is expanded to the user's home. The scan is recursive up
to depth 4 (so `Wallpapers/artist/series/file.png` works; deeper is
silently skipped). Supported extensions: `.png .jpg .jpeg .webp .bmp`
for images, `.mp4 .webm .mkv .mov .gif` for video.

Add multiple roots for a split library:

```json
"library": {
  "roots": [
    "~/Pictures/Wallpapers",
    "/mnt/art/wallpapers",
    "~/Downloads/tmp-wallpapers"
  ]
}
```

## Environment overrides

Wallforge honours a handful of XDG env vars. You rarely need to set
them manually — systemd/the shell already export them.

| Var               | Used for                                          |
|-------------------|---------------------------------------------------|
| `XDG_CONFIG_HOME` | config location (fallback `~/.config`)            |
| `XDG_STATE_HOME`  | `last.json`, `pending.json`, `workspaces.json` (fallback `~/.local/state`) |
| `XDG_RUNTIME_DIR` | Hyprland event socket lookup (workspace daemon)   |
| `HYPRLAND_INSTANCE_SIGNATURE` | Hyprland socket path piece              |

`pending.json` exists only while a Paused-mode `wallforge apply` (or
web-UI Apply) has queued a wallpaper for the next resume. The
watchdog and `wallforge resume` consume it (delete it on success);
the next resume after a successful render reads `last.json` again.

## Home Manager options (all opt-in)

```nix
programs.wallforge = {
  enable = true;                        # install binary + completions

  default = "3682370294";               # apply on login (mutually exclusive with shuffle)

  shuffle = {
    enable   = true;                    # wallforge.service = shuffle loop
    ids      = [ "1116273880" ];        # explicit playlist
    type     = "video";                 # or pick by type when ids=[]
    interval = "15min";                 # Go duration: 30s, 5m, 1h
  };

  serve = {
    enable = true;                      # wallforge-serve.service
    addr   = "127.0.0.1:7777";          # bind address
  };

  resume = {
    enable = true;                      # wallforge-resume.service (oneshot)
  };

  workspace = {
    enable = true;                      # wallforge-workspace.service
  };

  watchdog = {
    enable = true;                      # wallforge-watchdog.service
  };

  completion.enable = true;             # bash/zsh/fish drop-ins (default true)

  settings = { /* see schema above */ };
};
```

`shuffle.enable = true` is mutually exclusive with `default`. When both
are set, `shuffle` wins (rotating a playlist subsumes setting a single
default).
